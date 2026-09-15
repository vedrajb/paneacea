//go:build windows

package terminal

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

type conptyProcess struct {
	mu      sync.Mutex
	input   *os.File
	output  *os.File
	pseudo  windows.Handle
	process windows.Handle
	thread  windows.Handle
	pid     uint32
	job     windows.Handle

	closeRequest sync.Once
	waitOnce     sync.Once
	resourceOnce sync.Once
	waitError    error
}

var updateProcThreadAttribute = windows.NewLazySystemDLL("kernel32.dll").NewProc("UpdateProcThreadAttribute")

func newConPTY(options Options) (backend, error) {
	executable, arguments, err := resolveCommand(options)
	if err != nil {
		return nil, err
	}
	workingDirectory := options.WorkingDirectory
	if workingDirectory == "" {
		workingDirectory, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("get working directory: %w", err)
		}
	}
	info, err := os.Stat(workingDirectory)
	if err != nil {
		return nil, fmt.Errorf("stat working directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("working directory is not a directory: %s", workingDirectory)
	}

	var inputRead windows.Handle
	var inputWrite windows.Handle
	var outputRead windows.Handle
	var outputWrite windows.Handle
	var pseudo windows.Handle
	var processHandle windows.Handle
	var threadHandle windows.Handle
	var jobHandle windows.Handle
	cleanup := func() {
		if jobHandle != 0 {
			_ = windows.CloseHandle(jobHandle)
			jobHandle = 0
		}
		if threadHandle != 0 {
			_ = windows.CloseHandle(threadHandle)
			threadHandle = 0
		}
		if processHandle != 0 {
			_ = windows.TerminateProcess(processHandle, 1)
			_ = windows.CloseHandle(processHandle)
			processHandle = 0
		}
		if pseudo != 0 {
			windows.ClosePseudoConsole(pseudo)
			pseudo = 0
		}
		for _, handle := range []*windows.Handle{&inputRead, &inputWrite, &outputRead, &outputWrite} {
			if *handle != 0 {
				_ = windows.CloseHandle(*handle)
				*handle = 0
			}
		}
	}
	success := false
	defer func() {
		if !success {
			cleanup()
		}
	}()

	if err = windows.CreatePipe(&inputRead, &inputWrite, nil, 0); err != nil {
		return nil, fmt.Errorf("create ConPTY input pipe: %w", err)
	}
	if err = windows.CreatePipe(&outputRead, &outputWrite, nil, 0); err != nil {
		return nil, fmt.Errorf("create ConPTY output pipe: %w", err)
	}
	if err = windows.SetHandleInformation(inputWrite, windows.HANDLE_FLAG_INHERIT, 0); err != nil {
		return nil, fmt.Errorf("configure ConPTY input pipe: %w", err)
	}
	if err = windows.SetHandleInformation(outputRead, windows.HANDLE_FLAG_INHERIT, 0); err != nil {
		return nil, fmt.Errorf("configure ConPTY output pipe: %w", err)
	}

	size := windows.Coord{X: int16(options.Columns), Y: int16(options.Rows)}
	if err = windows.CreatePseudoConsole(size, inputRead, outputWrite, 0, &pseudo); err != nil {
		return nil, fmt.Errorf("create pseudo console: %w", err)
	}
	_ = windows.CloseHandle(inputRead)
	inputRead = 0
	_ = windows.CloseHandle(outputWrite)
	outputWrite = 0

	attributes, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return nil, fmt.Errorf("create process attribute list: %w", err)
	}
	defer attributes.Delete()
	if err = updatePseudoConsoleAttribute(attributes.List(), pseudo); err != nil {
		return nil, fmt.Errorf("attach pseudo console to process: %w", err)
	}

	applicationName, err := windows.UTF16PtrFromString(executable)
	if err != nil {
		return nil, fmt.Errorf("encode executable path: %w", err)
	}
	commandLine, err := windows.UTF16PtrFromString(composeCommandLine(executable, arguments))
	if err != nil {
		return nil, fmt.Errorf("encode command line: %w", err)
	}
	currentDirectory, err := windows.UTF16PtrFromString(workingDirectory)
	if err != nil {
		return nil, fmt.Errorf("encode working directory: %w", err)
	}

	startup := windows.StartupInfoEx{}
	startup.Cb = uint32(unsafe.Sizeof(startup))
	startup.Flags = windows.STARTF_USESTDHANDLES
	startup.ProcThreadAttributeList = attributes.List()
	var processInfo windows.ProcessInformation
	flags := uint32(windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_UNICODE_ENVIRONMENT | windows.CREATE_SUSPENDED)
	environment, err := environmentBlock(options.Environment)
	if err != nil {
		return nil, err
	}
	if err = windows.CreateProcess(
		applicationName,
		commandLine,
		nil,
		nil,
		false,
		flags,
		&environment[0],
		currentDirectory,
		&startup.StartupInfo,
		&processInfo,
	); err != nil {
		return nil, fmt.Errorf("start terminal process: %w", err)
	}
	processHandle = processInfo.Process
	threadHandle = processInfo.Thread
	jobHandle, err = windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("create terminal job: %w", err)
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err = windows.SetInformationJobObject(jobHandle, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return nil, fmt.Errorf("configure terminal job: %w", err)
	}
	if err = windows.AssignProcessToJobObject(jobHandle, processHandle); err != nil {
		return nil, fmt.Errorf("assign terminal job: %w", err)
	}
	if _, err = windows.ResumeThread(threadHandle); err != nil {
		return nil, fmt.Errorf("resume terminal process: %w", err)
	}
	_ = windows.CloseHandle(threadHandle)
	threadHandle = 0

	input := os.NewFile(uintptr(inputWrite), "conpty-input")
	output := os.NewFile(uintptr(outputRead), "conpty-output")
	if input == nil || output == nil {
		if input != nil {
			_ = input.Close()
		}
		if output != nil {
			_ = output.Close()
		}
		return nil, fmt.Errorf("wrap ConPTY pipes")
	}
	inputWrite = 0
	outputRead = 0
	session := &conptyProcess{
		input:   input,
		output:  output,
		pseudo:  pseudo,
		process: processHandle,
		pid:     processInfo.ProcessId,
		job:     jobHandle,
	}
	pseudo = 0
	processHandle = 0
	jobHandle = 0
	success = true
	return session, nil
}

func updatePseudoConsoleAttribute(list *windows.ProcThreadAttributeList, pseudo windows.Handle) error {
	result, _, callError := updateProcThreadAttribute.Call(
		uintptr(unsafe.Pointer(list)),
		0,
		windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE,
		uintptr(pseudo),
		unsafe.Sizeof(pseudo),
		0,
		0,
	)
	if result != 0 {
		return nil
	}
	if callError != nil && callError != windows.ERROR_SUCCESS {
		return callError
	}
	return windows.ERROR_INVALID_PARAMETER
}

func environmentBlock(overrides map[string]string) ([]uint16, error) {
	values := map[string]string{}
	noColorOverride := false
	for key := range overrides {
		if strings.EqualFold(key, "NO_COLOR") {
			noColorOverride = true
			break
		}
	}
	for _, entry := range os.Environ() {
		if index := strings.Index(entry, "="); index > 0 {
			key := entry[:index]
			if strings.EqualFold(key, "NO_COLOR") && !noColorOverride {
				continue
			}
			values[strings.ToUpper(key)] = entry[index+1:]
		}
	}
	for key, value := range overrides {
		if key == "" || strings.ContainsAny(key, "=\x00") || strings.ContainsRune(value, 0) {
			return nil, fmt.Errorf("invalid environment entry")
		}
		values[strings.ToUpper(key)] = value
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := []uint16{}
	for _, key := range keys {
		entry, err := windows.UTF16FromString(key + "=" + values[key])
		if err != nil {
			return nil, err
		}
		result = append(result, entry...)
	}
	return append(result, 0), nil
}

func resolveCommand(options Options) (string, []string, error) {
	executable := options.Executable
	arguments := options.Arguments
	if executable == "" {
		executable, arguments = defaultShell()
		if options.Arguments != nil {
			arguments = options.Arguments
		}
	}
	resolved, err := exec.LookPath(executable)
	if err != nil {
		return "", nil, fmt.Errorf("find shell %q: %w", executable, err)
	}
	return resolved, arguments, nil
}

func defaultShell() (string, []string) {
	gitBash := filepath.Join(os.Getenv("ProgramFiles"), "Git", "bin", "bash.exe")
	if resolved, err := exec.LookPath(gitBash); err == nil {
		return resolved, []string{"--login", "-i"}
	}
	for _, executable := range []string{"pwsh.exe", "powershell.exe"} {
		if resolved, err := exec.LookPath(executable); err == nil {
			return resolved, []string{"-NoLogo", "-NoProfile"}
		}
	}
	return "cmd.exe", []string{}
}

func composeCommandLine(executable string, arguments []string) string {
	parts := make([]string, 0, len(arguments)+1)
	parts = append(parts, quoteCommandLineArgument(executable))
	for _, argument := range arguments {
		parts = append(parts, quoteCommandLineArgument(argument))
	}
	return strings.Join(parts, " ")
}

func quoteCommandLineArgument(value string) string {
	if value != "" && !strings.ContainsAny(value, " \t\"") {
		return value
	}
	var builder strings.Builder
	builder.WriteByte('"')
	backslashes := 0
	for index := 0; index < len(value); index++ {
		switch value[index] {
		case '\\':
			backslashes++
		case '"':
			builder.WriteString(strings.Repeat("\\", backslashes*2+1))
			builder.WriteByte('"')
			backslashes = 0
		default:
			if backslashes > 0 {
				builder.WriteString(strings.Repeat("\\", backslashes))
				backslashes = 0
			}
			builder.WriteByte(value[index])
		}
	}
	if backslashes > 0 {
		builder.WriteString(strings.Repeat("\\", backslashes*2))
	}
	builder.WriteByte('"')
	return builder.String()
}

func (p *conptyProcess) Read(data []byte) (int, error) {
	p.mu.Lock()
	output := p.output
	p.mu.Unlock()
	if output == nil {
		return 0, io.EOF
	}
	return output.Read(data)
}

func (p *conptyProcess) Write(data []byte) (int, error) {
	p.mu.Lock()
	input := p.input
	p.mu.Unlock()
	if input == nil {
		return 0, io.ErrClosedPipe
	}
	return input.Write(data)
}

func (p *conptyProcess) Resize(columns, rows uint16) error {
	if err := ValidateSize(columns, rows); err != nil {
		return err
	}
	p.mu.Lock()
	pseudo := p.pseudo
	p.mu.Unlock()
	if pseudo == 0 {
		return io.ErrClosedPipe
	}
	return windows.ResizePseudoConsole(pseudo, windows.Coord{X: int16(columns), Y: int16(rows)})
}

func (p *conptyProcess) Wait() error {
	p.waitOnce.Do(func() {
		p.mu.Lock()
		processHandle := p.process
		p.mu.Unlock()
		if processHandle != 0 {
			result, err := windows.WaitForSingleObject(processHandle, windows.INFINITE)
			if err != nil {
				p.waitError = err
			} else if result != windows.WAIT_OBJECT_0 {
				p.waitError = fmt.Errorf("wait for terminal process returned 0x%x", result)
			}
		}
		p.releaseResources()
	})
	return p.waitError
}

func (p *conptyProcess) Close() error {
	p.closeRequest.Do(func() {
		p.mu.Lock()
		processHandle := p.process
		jobHandle := p.job
		p.job = 0
		pseudo := p.pseudo
		p.pseudo = 0
		input := p.input
		p.input = nil
		output := p.output
		p.output = nil
		p.mu.Unlock()
		if jobHandle != 0 {
			_ = windows.CloseHandle(jobHandle)
		}
		if processHandle != 0 {
			_ = windows.TerminateProcess(processHandle, 1)
		}
		if pseudo != 0 {
			windows.ClosePseudoConsole(pseudo)
		}
		if input != nil {
			_ = input.Close()
		}
		if output != nil {
			_ = output.Close()
		}
	})
	return p.Wait()
}

func (p *conptyProcess) PID() uint32 {
	return p.pid
}

func (p *conptyProcess) releaseResources() {
	p.resourceOnce.Do(func() {
		p.mu.Lock()
		input := p.input
		p.input = nil
		output := p.output
		p.output = nil
		pseudo := p.pseudo
		p.pseudo = 0
		processHandle := p.process
		p.process = 0
		threadHandle := p.thread
		jobHandle := p.job
		p.job = 0
		p.thread = 0
		p.mu.Unlock()
		if jobHandle != 0 {
			_ = windows.CloseHandle(jobHandle)
		}
		if input != nil {
			_ = input.Close()
		}
		if output != nil {
			_ = output.Close()
		}
		if pseudo != 0 {
			windows.ClosePseudoConsole(pseudo)
		}
		if threadHandle != 0 {
			_ = windows.CloseHandle(threadHandle)
		}
		if processHandle != 0 {
			_ = windows.CloseHandle(processHandle)
		}
	})
}
