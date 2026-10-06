package agents

import "testing"

func TestScriptHostDetection(t *testing.T) {
	for executable, want := range map[string]bool{
		"node.exe": true, "C:\\Program Files\\nodejs\\NODE.EXE": true, "bun.exe": true, "deno": true,
		"python.exe": true, "pythonw.exe": true, "py.exe": true,
		"codex.exe": false, "cmd.exe": false, "nodemon.exe": false,
	} {
		if got := IsScriptHost(executable); got != want {
			t.Fatalf("%s: got %v want %v", executable, got, want)
		}
	}
}

func TestCommandLineDetectsScriptHostedAgents(t *testing.T) {
	for commandLine, want := range map[string]string{
		`"C:\Program Files\nodejs\node.exe" "C:\Users\u\AppData\Roaming\npm\node_modules\@openai\codex\bin\codex.js"`: "codex",
		`node C:\Users\u\AppData\Roaming\npm\node_modules\@anthropic-ai\claude-code\cli.js --resume`:                  "claude",
		`node.exe C:/npm/node_modules/@google/gemini-cli/dist/index.js`:                                               "gemini",
		`node "C:\npm\node_modules\@qwen-code\qwen-code\cli.js"`:                                                      "qwen",
		`node C:\npm\node_modules\@earendil-works\pi-coding-agent\dist\cli.js`:                                        "pi",
		`node C:\npm\node_modules\@sourcegraph\amp\dist\main.js`:                                                      "amp",
		`bun C:\Users\u\.bun\install\global\node_modules\opencode-ai\bin\opencode`:                                    "opencode",
		`deno run -A npm:@google/gemini-cli@latest`:                                                                   "gemini",
		`deno run -A npm:@kilocode/cli`:                                                                               "kilocode",
		`node C:\npm\node_modules\cline\bin\cline.js`:                                                                 "cline",
		`python.exe -m aider --model x`:                                                                               "aider",
		`"C:\Python312\python.exe" "C:\Python312\Lib\site-packages\aider\main.py"`:                                    "aider",
		`python -m openhands.cli`:                                                                                     "",
		`node C:\npm\node_modules\@openai\codex-helper\index.js`:                                                      "",
		`node C:\npm\node_modules\clinex\index.js`:                                                                    "",
		`node C:\work\pi\amp\agent.js`:                                                                                "",
		`node server.js --name codex`:                                                                                 "",
		`python -m aiderx`:                                                                                            "",
		``:                                                                                                            "",
	} {
		if got := DetectCommandLine(commandLine); got != want {
			t.Fatalf("%s: got %q want %q", commandLine, got, want)
		}
	}
}
