use crate::{
    logging,
    model::{
        Pane, DEFAULT_TERMINAL_COLUMNS, DEFAULT_TERMINAL_HISTORY_LINES, DEFAULT_TERMINAL_ROWS,
    },
    persistence::SavedHistory,
};
use anyhow::{Context, Result};
use portable_pty::{native_pty_system, Child, CommandBuilder, MasterPty, PtySize};
use std::{
    collections::BTreeMap,
    io::{Read, Write},
    path::{Path, PathBuf},
    sync::{Arc, Mutex},
};
use tokio::sync::broadcast;

const RESTART_DIVIDER: &[u8] = b"\x1b[90m\r\n--- Paneacea restored terminal history; the previous process ended and a new shell was started ---\x1b[0m\r\n";

pub struct Output {
    pub parser: vt100::Parser,
    pub sender: broadcast::Sender<Vec<u8>>,
    pub exited: bool,
    pub title: String,
    pub cwd: Option<String>,
    pub dirty: bool,
    pub viewport_offset: usize,
    pub restored: bool,
}
pub struct Terminal {
    pub master: Box<dyn MasterPty + Send>,
    pub writer: Arc<Mutex<Box<dyn Write + Send>>>,
    pub child: Box<dyn Child + Send + Sync>,
    pub output: Arc<Mutex<Output>>,
    integration_file: Option<PathBuf>,
}
impl Terminal {
    pub fn launch(
        pane: &mut Pane,
        pipe: &str,
        history: Option<SavedHistory>,
        history_limit: usize,
    ) -> Result<Self> {
        logging::info(
            "pty.launch.start",
            &format!(
                "pane_id={} executable={} cwd={} history_present={} history_limit={history_limit}",
                pane.id,
                pane.executable,
                pane.current_working_directory,
                history.is_some()
            ),
        );
        let (rows, columns) = history
            .as_ref()
            .map(|value| (value.rows, value.columns))
            .unwrap_or((pane.terminal_rows, pane.terminal_columns));
        let rows = valid_rows(rows);
        let columns = valid_columns(columns);
        pane.terminal_rows = rows;
        pane.terminal_columns = columns;
        let pair = native_pty_system().openpty(PtySize {
            rows,
            cols: columns,
            pixel_width: 0,
            pixel_height: 0,
        })?;
        let mut command = CommandBuilder::new(&pane.executable);
        let mut arguments = pane.arguments.clone();
        let mut environment = pane.environment.clone();
        let bash_integration = bash_integration_enabled(&pane.executable, &pane.arguments);
        configure_shell_integration(
            &pane.executable,
            &pane.arguments,
            &mut arguments,
            &mut environment,
        );
        let integration_file = if bash_integration && !has_bash_rcfile(&pane.arguments) {
            arguments.insert(0, "--rcfile".into());
            let path = create_bash_rcfile(!has_bash_noprofile(&pane.arguments))?;
            arguments.insert(1, path.to_string_lossy().into_owned());
            arguments.insert(2, "--noprofile".into());
            Some(path)
        } else {
            None
        };
        logging::info(
            "pty.launch.command",
            &format!(
                "pane_id={} argument_count={} environment_count={} bash_rcfile={} rows={rows} columns={columns}",
                pane.id,
                arguments.len(),
                environment.len(),
                integration_file.is_some()
            ),
        );
        command.args(&arguments);
        command.cwd(&pane.current_working_directory);
        for (key, value) in &environment {
            command.env(key, value);
        }
        command.env("PANEACEA_PIPE", pipe);
        command.env("PANEACEA_PANE_ID", &pane.id);
        command.env("PANEACEA_WORKSPACE_ID", &pane.workspace_id);
        let child = match pair
            .slave
            .spawn_command(command)
            .context("could not launch terminal")
        {
            Ok(child) => child,
            Err(error) => {
                logging::error(
                    "pty.launch.failed",
                    &format!("pane_id={} error={error:#}", pane.id),
                );
                remove_integration_file(integration_file.as_ref());
                return Err(error);
            }
        };
        pane.pid = child.process_id();
        pane.generation = crate::model::id();
        pane.error = None;
        drop(pair.slave);
        let mut reader = match pair.master.try_clone_reader() {
            Ok(reader) => reader,
            Err(error) => {
                remove_integration_file(integration_file.as_ref());
                return Err(error.into());
            }
        };
        let writer = match pair.master.take_writer() {
            Ok(writer) => Arc::new(Mutex::new(writer)),
            Err(error) => {
                remove_integration_file(integration_file.as_ref());
                return Err(error.into());
            }
        };
        let (sender, _) = broadcast::channel(256);
        let parser_history = history_limit
            .max(DEFAULT_TERMINAL_HISTORY_LINES)
            .min(25_000);
        let mut parser = vt100::Parser::new(rows, columns, parser_history);
        if let Some(history) = &history {
            parser.process(&history.data);
            parser.process(RESTART_DIVIDER);
            parser.set_scrollback(0);
        }
        let viewport_offset = 0;
        let output = Arc::new(Mutex::new(Output {
            parser,
            sender,
            exited: false,
            title: pane.title.clone(),
            cwd: Some(pane.current_working_directory.clone()),
            dirty: false,
            viewport_offset,
            restored: history.is_some(),
        }));
        logging::info(
            "pty.launch.ready",
            &format!(
                "pane_id={} pid={} generation={} restored={}",
                pane.id,
                pane.pid
                    .map_or_else(|| "none".into(), |pid| pid.to_string()),
                pane.generation,
                history.is_some()
            ),
        );
        let thread_output = output.clone();
        let thread_writer = writer.clone();
        let log_pane_id = pane.id.clone();
        std::thread::spawn(move || {
            let mut buffer = [0u8; 8192];
            let mut queries = crate::queries::Queries::default();
            loop {
                match reader.read(&mut buffer) {
                    Ok(0) | Err(_) => break,
                    Ok(count) => {
                        let mut output = thread_output.lock().unwrap();
                        let (data, replies, title, cwd) =
                            queries.process_with_title(&buffer[..count], &mut output.parser);
                        if let Some(title) = title {
                            logging::info(
                                "pty.metadata.title",
                                &format!("pane_id={log_pane_id} title={title}"),
                            );
                            output.title = title;
                            output.dirty = true;
                        }
                        if let Some(cwd) = cwd {
                            logging::info(
                                "pty.metadata.cwd",
                                &format!("pane_id={log_pane_id} cwd={cwd}"),
                            );
                            output.cwd = Some(cwd);
                            output.dirty = true;
                        }
                        if !data.is_empty() {
                            output.dirty = true;
                            let _ = output.sender.send(data);
                        }
                        drop(output);
                        if !replies.is_empty() {
                            let mut writer = thread_writer.lock().unwrap();
                            if writer
                                .write_all(&replies)
                                .and_then(|_| writer.flush())
                                .is_err()
                            {
                                break;
                            }
                        }
                    }
                }
            }
            let mut output = thread_output.lock().unwrap();
            output.exited = true;
            logging::info("pty.reader.exited", &format!("pane_id={log_pane_id}"));
            let _ = output.sender.send(Vec::new());
        });
        Ok(Self {
            master: pair.master,
            writer,
            child,
            output,
            integration_file,
        })
    }
    pub fn resize(&mut self, rows: u16, columns: u16) -> Result<()> {
        anyhow::ensure!(
            (1..=500).contains(&rows) && (1..=1000).contains(&columns),
            "invalid terminal dimensions"
        );
        self.master.resize(PtySize {
            rows,
            cols: columns,
            pixel_width: 0,
            pixel_height: 0,
        })?;
        let mut output = self.output.lock().unwrap();
        output.parser.set_size(rows, columns);
        output.viewport_offset = output.parser.screen().scrollback();
        output.dirty = true;
        Ok(())
    }

    pub fn set_viewport(&mut self, offset: usize) -> usize {
        let mut output = self.output.lock().unwrap();
        output.parser.set_scrollback(offset);
        output.viewport_offset = output.parser.screen().scrollback();
        output.viewport_offset
    }
    pub fn write(&mut self, data: &[u8]) -> Result<()> {
        let mut writer = self.writer.lock().unwrap();
        writer.write_all(data)?;
        writer.flush()?;
        Ok(())
    }
    pub fn close(&mut self) {
        let _ = self.child.kill();
        let _ = self.child.wait();
        remove_integration_file(self.integration_file.take().as_ref());
    }
}
impl Drop for Terminal {
    fn drop(&mut self) {
        self.close();
    }
}

pub fn history_snapshot(output: &mut Output, line_limit: usize) -> SavedHistory {
    let parser = &mut output.parser;
    let viewport_offset = output.viewport_offset.min(parser.screen().scrollback());
    parser.set_scrollback(usize::MAX);
    let max_offset = parser.screen().scrollback();
    let (rows, columns) = parser.screen().size();
    let total_rows = max_offset.saturating_add(usize::from(rows));
    let mut saved_rows = Vec::with_capacity(total_rows);
    for global_row in 0..total_rows {
        let (scrollback, row) = if global_row < max_offset {
            (max_offset - global_row, 0)
        } else {
            (0, global_row - max_offset)
        };
        parser.set_scrollback(scrollback);
        let screen = parser.screen();
        if let Some(formatted) = screen.rows_formatted(0, columns).nth(row) {
            let blank = screen
                .rows(0, columns)
                .nth(row)
                .is_none_or(|contents| contents.trim_end().is_empty());
            saved_rows.push((formatted, screen.row_wrapped(row as u16), blank));
        }
    }
    while saved_rows.last().is_some_and(|(_, _, blank)| *blank) {
        saved_rows.pop();
    }
    let start = saved_rows.len().saturating_sub(line_limit);
    let mut data = b"\x1b[2J\x1b[H\x1b[0m".to_vec();
    let saved_rows = &saved_rows[start..];
    for (index, (formatted, wrapped, _)) in saved_rows.iter().enumerate() {
        data.extend(formatted);
        if index + 1 < saved_rows.len() && !wrapped {
            data.extend_from_slice(b"\r\n");
        }
    }
    parser.set_scrollback(viewport_offset);
    SavedHistory {
        data,
        rows,
        columns,
    }
}

fn valid_rows(rows: u16) -> u16 {
    if (1..=500).contains(&rows) {
        rows
    } else {
        DEFAULT_TERMINAL_ROWS
    }
}

fn valid_columns(columns: u16) -> u16 {
    if (1..=1000).contains(&columns) {
        columns
    } else {
        DEFAULT_TERMINAL_COLUMNS
    }
}

fn bash_integration_enabled(executable: &str, original_arguments: &[String]) -> bool {
    let name = Path::new(executable)
        .file_stem()
        .and_then(|value| value.to_str())
        .unwrap_or_default()
        .to_ascii_lowercase();
    name == "bash"
        && !original_arguments.iter().any(|argument| {
            matches!(
                argument.to_ascii_lowercase().as_str(),
                "-c" | "--command" | "--norc"
            )
        })
}

fn has_bash_rcfile(arguments: &[String]) -> bool {
    arguments.iter().any(|argument| {
        let argument = argument.to_ascii_lowercase();
        matches!(argument.as_str(), "--rcfile" | "--init-file")
            || argument.starts_with("--rcfile=")
            || argument.starts_with("--init-file=")
    })
}

fn has_bash_noprofile(arguments: &[String]) -> bool {
    arguments.iter().any(|argument| {
        matches!(
            argument.to_ascii_lowercase().as_str(),
            "--noprofile" | "--no-profile"
        )
    })
}

fn create_bash_rcfile(source_profile: bool) -> Result<PathBuf> {
    let path = std::env::temp_dir().join(format!("paneacea-bash-{}.rc", crate::model::id()));
    std::fs::write(&path, bash_rcfile_contents(source_profile))
        .with_context(|| format!("could not create Bash integration file {}", path.display()))?;
    Ok(path)
}

fn remove_integration_file(path: Option<&PathBuf>) {
    if let Some(path) = path {
        let _ = std::fs::remove_file(path);
    }
}

fn bash_rcfile_contents(source_profile: bool) -> String {
    let profile = if source_profile {
        r#"if [ -f /etc/profile ]; then
    . /etc/profile
fi
for profile in "$HOME/.bash_profile" "$HOME/.bash_login" "$HOME/.profile"; do
    if [ -f "$profile" ]; then
        . "$profile"
        break
    fi
done
"#
    } else {
        ""
    };
    format!(
        r#"{profile}__paneacea_prompt_command() {{
    printf '\033]7;file:///%s\a' "$(pwd -W 2>/dev/null || pwd)"
}}
case "${{PROMPT_COMMAND:-}}" in
    *__paneacea_prompt_command*) ;;
    *) PROMPT_COMMAND="__paneacea_prompt_command${{PROMPT_COMMAND:+;$PROMPT_COMMAND}}" ;;
esac
"#
    )
}

fn configure_shell_integration(
    executable: &str,
    original_arguments: &[String],
    arguments: &mut Vec<String>,
    environment: &mut BTreeMap<String, String>,
) {
    let name = Path::new(executable)
        .file_stem()
        .and_then(|value| value.to_str())
        .unwrap_or_default()
        .to_ascii_lowercase();
    if matches!(name.as_str(), "pwsh" | "powershell") {
        if original_arguments.iter().any(|argument| {
            matches!(
                argument.to_ascii_lowercase().as_str(),
                "-command" | "-c" | "-file" | "-f" | "-encodedcommand" | "-ec" | "-noninteractive"
            )
        }) {
            return;
        }
        if !arguments
            .iter()
            .any(|argument| argument.eq_ignore_ascii_case("-noexit"))
        {
            arguments.push("-NoExit".into());
        }
        arguments.push("-Command".into());
        arguments.push(
            "$global:PaneaceaOriginalPrompt = (Get-Command prompt -CommandType Function -ErrorAction SilentlyContinue).ScriptBlock; function global:prompt { $path = (Get-Location).Path; $uri = [System.Uri]::new($path).AbsoluteUri; [Console]::Write(([char]27 + ']7;' + $uri + [char]7)); if ($null -ne $global:PaneaceaOriginalPrompt) { & $global:PaneaceaOriginalPrompt } else { \"PS $path> \" } }".into(),
        );
    } else if name == "bash"
        && !original_arguments.iter().any(|argument| {
            matches!(
                argument.to_ascii_lowercase().as_str(),
                "-c" | "--command" | "--norc"
            )
        })
    {
        let hook = r#"printf '\033]7;file:///%s\a' "$(pwd -W 2>/dev/null || pwd)""#;
        let prompt_command = environment
            .get("PROMPT_COMMAND")
            .filter(|value| !value.trim().is_empty())
            .map(|value| format!("{hook};{value}"))
            .unwrap_or_else(|| hook.into());
        environment.insert("PROMPT_COMMAND".into(), prompt_command);
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn shell_integration_preserves_existing_arguments_and_prompt_command() {
        let mut arguments = vec!["--login".into(), "-i".into()];
        let original = arguments.clone();
        let mut environment =
            BTreeMap::from([(String::from("PROMPT_COMMAND"), String::from("history -a"))]);
        configure_shell_integration("bash.exe", &original, &mut arguments, &mut environment);
        assert_eq!(&arguments[..2], &original[..]);
        assert!(environment["PROMPT_COMMAND"].contains("history -a"));
        assert!(environment["PROMPT_COMMAND"].contains("pwd -W"));
    }

    #[test]
    fn bash_rcfile_preserves_startup_and_appends_cwd_hook() {
        let contents = bash_rcfile_contents(true);
        assert!(contents.contains("/etc/profile"));
        assert!(contents.contains("$HOME/.bash_profile"));
        assert!(contents.contains("__paneacea_prompt_command"));
        assert!(contents.contains("PROMPT_COMMAND"));
    }

    #[test]
    fn history_snapshot_is_bounded() {
        let (sender, _) = broadcast::channel(4);
        let mut output = Output {
            parser: vt100::Parser::new(3, 20, 20),
            sender,
            exited: false,
            title: String::new(),
            cwd: None,
            dirty: true,
            viewport_offset: 0,
            restored: false,
        };
        output
            .parser
            .process(b"one\r\ntwo\r\nthree\r\nfour\r\nfive");
        let history = history_snapshot(&mut output, 3);
        assert!(history.data.len() > 0);
        assert_eq!(history.rows, 3);
        assert_eq!(history.columns, 20);
    }
}
