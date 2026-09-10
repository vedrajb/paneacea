use crate::{
    logging,
    model::*,
    persistence::Store,
    pty::{history_snapshot, Terminal},
};
use anyhow::{anyhow, bail, ensure, Result};
use serde_json::{json, Value};
use std::{collections::BTreeMap, path::Path};

pub struct Runtime {
    pub state: State,
    pub terminals: BTreeMap<String, Terminal>,
    store: Store,
    pipe: String,
}
fn required<'a>(value: &'a Value, key: &str) -> Result<&'a str> {
    value[key]
        .as_str()
        .filter(|s| !s.trim().is_empty())
        .ok_or_else(|| anyhow!("missing {key}"))
}
fn directory(value: &str) -> Result<String> {
    let path = Path::new(value);
    ensure!(
        path.is_absolute() && path.is_dir(),
        "root directory must be an existing absolute directory"
    );
    Ok(value.into())
}
impl Runtime {
    pub fn open(path: &Path, pipe: String) -> Result<Self> {
        logging::info(
            "runtime.restore.start",
            &format!("database={} pipe={pipe}", path.display()),
        );
        let store = Store::open(path)?;
        let state = store.load()?;
        let history_limit = state
            .settings
            .get(TERMINAL_HISTORY_SETTING)
            .and_then(Value::as_u64)
            .and_then(|value| usize::try_from(value).ok())
            .filter(|value| valid_history_limit(*value))
            .unwrap_or(DEFAULT_TERMINAL_HISTORY_LINES);
        logging::info(
            "runtime.restore.settings",
            &format!(
                "history_limit={} persisted_panes={} configured_history_setting={}",
                history_limit,
                state.panes.len(),
                state
                    .settings
                    .get(TERMINAL_HISTORY_SETTING)
                    .map(Value::to_string)
                    .unwrap_or_else(|| "missing".into())
            ),
        );
        let mut histories = BTreeMap::new();
        if history_limit > 0 {
            for pane_id in state.panes.keys() {
                if let Some(history) = store.load_history(pane_id)? {
                    histories.insert(pane_id.clone(), history);
                }
            }
        }
        logging::info(
            "runtime.restore.histories",
            &format!(
                "history_rows_loaded={} history_enabled={}",
                histories.len(),
                history_limit > 0
            ),
        );
        let mut runtime = Self {
            state,
            terminals: BTreeMap::new(),
            store,
            pipe,
        };
        for pane in runtime.state.panes.values_mut() {
            let history = histories.remove(&pane.id);
            let restored = history.is_some();
            logging::info(
                "pane.restore.start",
                &format!(
                    "pane_id={} tab_id={} history_present={} rows={} columns={} cwd={}",
                    pane.id,
                    pane.tab_id,
                    history.is_some(),
                    history
                        .as_ref()
                        .map(|value| value.rows)
                        .unwrap_or(pane.terminal_rows),
                    history
                        .as_ref()
                        .map(|value| value.columns)
                        .unwrap_or(pane.terminal_columns),
                    pane.current_working_directory
                ),
            );
            if let Some(history) = &history {
                pane.terminal_rows = history.rows;
                pane.terminal_columns = history.columns;
            }
            pane.pid = None;
            if pane.title.trim().is_empty() {
                pane.title = shell_title(&pane.executable);
            }
            match Terminal::launch(pane, &runtime.pipe, history, history_limit) {
                Ok(terminal) => {
                    logging::info(
                        "pane.restore.complete",
                        &format!(
                            "pane_id={} pid={} generation={} restored={}",
                            pane.id,
                            pane.pid
                                .map_or_else(|| "none".into(), |pid| pid.to_string()),
                            pane.generation,
                            restored
                        ),
                    );
                    runtime.terminals.insert(pane.id.clone(), terminal);
                }
                Err(error) => {
                    logging::error(
                        "pane.restore.failed",
                        &format!("pane_id={} error={error:#}", pane.id),
                    );
                    pane.error = Some(error.to_string())
                }
            }
        }
        runtime.store.save(&runtime.state)?;
        if history_limit == 0 {
            runtime.store.clear_history()?;
        }
        logging::info(
            "runtime.restore.complete",
            &format!(
                "workspaces={} panes={} terminals={} orphaned_history_rows={}",
                runtime.state.workspaces.len(),
                runtime.state.panes.len(),
                runtime.terminals.len(),
                histories.len()
            ),
        );
        Ok(runtime)
    }
    fn workspace(&mut self, id: &str) -> Result<&mut Workspace> {
        self.state
            .workspaces
            .iter_mut()
            .find(|w| w.id == id)
            .ok_or_else(|| anyhow!("workspace not found"))
    }
    fn tab(&mut self, id: &str) -> Result<&mut Tab> {
        self.state
            .workspaces
            .iter_mut()
            .flat_map(|w| &mut w.tabs)
            .find(|t| t.id == id)
            .ok_or_else(|| anyhow!("tab not found"))
    }
    fn configured_shell(&self) -> Option<(String, Vec<String>)> {
        let setting = self.state.settings.get("defaultShell")?;
        let executable = setting["executable"].as_str()?.trim();
        if executable.is_empty() {
            return None;
        }
        let arguments = setting["arguments"]
            .as_array()
            .map(|values| {
                values
                    .iter()
                    .filter_map(|value| value.as_str().map(ToOwned::to_owned))
                    .collect()
            })
            .unwrap_or_else(|| vec!["-NoLogo".into()]);
        Some((executable.into(), arguments))
    }
    fn sync_output_metadata(&mut self) -> bool {
        let mut changed = false;
        for (pane_id, terminal) in &self.terminals {
            let output = terminal.output.lock().unwrap();
            if let Some(pane) = self.state.panes.get_mut(pane_id) {
                if pane.title != output.title {
                    logging::info(
                        "pane.metadata.title",
                        &format!("pane_id={pane_id} title={}", output.title),
                    );
                    pane.title = output.title.clone();
                    changed = true;
                }
                if let Some(cwd) = &output.cwd {
                    if Path::new(cwd).is_dir() && pane.current_working_directory != *cwd {
                        logging::info("pane.metadata.cwd", &format!("pane_id={pane_id} cwd={cwd}"));
                        pane.current_working_directory = cwd.clone();
                        changed = true;
                    }
                }
            }
        }
        changed
    }
    fn history_limit(&self) -> usize {
        self.state
            .settings
            .get(TERMINAL_HISTORY_SETTING)
            .and_then(Value::as_u64)
            .and_then(|value| usize::try_from(value).ok())
            .filter(|value| valid_history_limit(*value))
            .unwrap_or(DEFAULT_TERMINAL_HISTORY_LINES)
    }
    fn mark_history_dirty(&mut self) {
        for terminal in self.terminals.values() {
            terminal.output.lock().unwrap().dirty = true;
        }
    }
    pub fn flush_persistence(&mut self) -> Result<()> {
        let metadata_changed = self.sync_output_metadata();
        if metadata_changed {
            logging::info("persistence.flush.metadata", "terminal metadata changed");
            self.store.save(&self.state)?;
        }
        let history_limit = self.history_limit();
        if history_limit == 0 {
            self.store.clear_history()?;
            return Ok(());
        }
        let mut pending = Vec::new();
        for (pane_id, terminal) in &self.terminals {
            let mut output = terminal.output.lock().unwrap();
            if output.dirty {
                pending.push((
                    pane_id.clone(),
                    history_snapshot(&mut output, history_limit),
                ));
            }
        }
        if !pending.is_empty() {
            logging::info(
                "persistence.flush.history",
                &format!(
                    "pending_panes={} history_limit={history_limit}",
                    pending.len()
                ),
            );
        }
        for (pane_id, history) in pending {
            self.store.save_history(&pane_id, &history)?;
            if let Some(terminal) = self.terminals.get(&pane_id) {
                terminal.output.lock().unwrap().dirty = false;
            }
        }
        Ok(())
    }
    fn new_pane(&mut self, workspace_id: &str, tab_id: &str, value: &Value) -> Result<Pane> {
        let root = self.workspace(workspace_id)?.root_directory.clone();
        let configured = self
            .configured_shell()
            .unwrap_or_else(|| ("powershell.exe".into(), vec!["-NoLogo".into()]));
        let explicit_executable = value["executable"].as_str();
        let executable = explicit_executable
            .map(ToOwned::to_owned)
            .unwrap_or_else(|| configured.0.clone());
        ensure!(!executable.trim().is_empty(), "executable cannot be empty");
        let arguments = if value.get("arguments").is_some() {
            serde_json::from_value(value["arguments"].clone())?
        } else if explicit_executable.is_none() {
            configured.1
        } else {
            vec!["-NoLogo".into()]
        };
        let title = shell_title(&executable);
        let environment = if value.get("environment").is_some() {
            serde_json::from_value(value["environment"].clone())?
        } else {
            BTreeMap::new()
        };
        Ok(Pane {
            id: id(),
            workspace_id: workspace_id.into(),
            tab_id: tab_id.into(),
            profile_id: "default".into(),
            executable,
            title,
            arguments,
            environment,
            initial_working_directory: root.clone(),
            current_working_directory: root,
            terminal_rows: DEFAULT_TERMINAL_ROWS,
            terminal_columns: DEFAULT_TERMINAL_COLUMNS,
            runtime_terminal_id: id(),
            pid: None,
            generation: id(),
            agent: None,
            error: None,
        })
    }
    fn close_pane(&mut self, pane_id: &str) -> Result<()> {
        let pane = self
            .state
            .panes
            .remove(pane_id)
            .ok_or_else(|| anyhow!("pane not found"))?;
        let workspace = self.workspace(&pane.workspace_id)?;
        if let Some(index) = workspace.tabs.iter().position(|t| t.id == pane.tab_id) {
            let tab = &mut workspace.tabs[index];
            if let Some(layout) = tab.root_layout_node.clone().remove(pane_id) {
                tab.root_layout_node = layout;
                if tab.active_pane_id == pane_id {
                    tab.active_pane_id = tab.root_layout_node.ids()[0].clone();
                }
            } else {
                workspace.tabs.remove(index);
                if workspace.active_tab_id.as_deref() == Some(&pane.tab_id) {
                    workspace.active_tab_id = workspace.tabs.first().map(|t| t.id.clone());
                }
            }
        }
        Ok(())
    }
    pub fn dispatch(&mut self, operation: &str, value: Value) -> Result<Value> {
        if self.sync_output_metadata() {
            self.store.save(&self.state)?;
        }
        match operation {
            "workspace.list" | "state.get" => {
                if operation == "workspace.list" {
                    logging::info(
                        "state.request.complete",
                        &format!(
                            "workspaces={} panes={} active_workspace={}",
                            self.state.workspaces.len(),
                            self.state.panes.len(),
                            self.state.active_workspace_id.as_deref().unwrap_or("none")
                        ),
                    );
                }
                return Ok(serde_json::to_value(&self.state)?);
            }
            "pane.sendInput" => {
                let pane_id = required(&value, "paneId")?;
                let data = value["data"]
                    .as_str()
                    .ok_or_else(|| anyhow!("missing data"))?;
                self.terminals
                    .get_mut(pane_id)
                    .ok_or_else(|| anyhow!("terminal unavailable"))?
                    .write(data.as_bytes())?;
                return Ok(json!({}));
            }
            "terminal.resize" => {
                let pane_id = required(&value, "paneId")?;
                let rows = u16::try_from(
                    value["rows"]
                        .as_u64()
                        .ok_or_else(|| anyhow!("missing rows"))?,
                )?;
                let columns = u16::try_from(
                    value["columns"]
                        .as_u64()
                        .ok_or_else(|| anyhow!("missing columns"))?,
                )?;
                self.terminals
                    .get_mut(pane_id)
                    .ok_or_else(|| anyhow!("terminal unavailable"))?
                    .resize(rows, columns)?;
                if let Some(pane) = self.state.panes.get_mut(pane_id) {
                    pane.terminal_rows = rows;
                    pane.terminal_columns = columns;
                }
                logging::info(
                    "terminal.resize",
                    &format!("pane_id={pane_id} rows={rows} columns={columns}"),
                );
                self.store.save(&self.state)?;
                return Ok(json!({}));
            }
            "terminal.viewport.set" => {
                let pane_id = required(&value, "paneId")?;
                let offset = usize::try_from(
                    value["offset"]
                        .as_u64()
                        .ok_or_else(|| anyhow!("missing offset"))?,
                )?;
                let actual = self
                    .terminals
                    .get_mut(pane_id)
                    .ok_or_else(|| anyhow!("terminal unavailable"))?
                    .set_viewport(offset);
                logging::info(
                    "terminal.viewport",
                    &format!("pane_id={pane_id} requested_offset={offset} actual_offset={actual}"),
                );
                return Ok(json!({"viewportOffset": actual}));
            }
            _ => {}
        }
        let previous = self.state.clone();
        let mut launched = Vec::new();
        let history_setting_changed =
            operation == "settings.set" && value["key"].as_str() == Some(TERMINAL_HISTORY_SETTING);
        let result = self.mutate(operation, &value, &mut launched).and_then(|_| {
            self.sync_output_metadata();
            self.store.save(&self.state)?;
            if history_setting_changed {
                self.mark_history_dirty();
                self.flush_persistence()?;
            }
            Ok(())
        });
        if let Err(error) = result {
            logging::error(
                "state.mutation.failed",
                &format!("operation={operation} error={error:#}"),
            );
            self.state = previous;
            for pane_id in launched {
                self.terminals.remove(&pane_id);
            }
            return Err(error);
        }
        self.terminals
            .retain(|pane_id, _| self.state.panes.contains_key(pane_id));
        logging::info(
            "state.mutation.complete",
            &format!(
                "operation={operation} workspaces={} panes={} terminals={}",
                self.state.workspaces.len(),
                self.state.panes.len(),
                self.terminals.len()
            ),
        );
        Ok(serde_json::to_value(&self.state)?)
    }
    fn mutate(&mut self, operation: &str, value: &Value, launched: &mut Vec<String>) -> Result<()> {
        match operation {
            "workspace.create" => {
                let workspace = Workspace {
                    id: id(),
                    name: required(value, "name")?.into(),
                    root_directory: directory(required(value, "rootDirectory")?)?,
                    tabs: vec![],
                    active_tab_id: None,
                    created_at: now(),
                    last_opened_at: now(),
                };
                self.state.active_workspace_id = Some(workspace.id.clone());
                self.state.workspaces.push(workspace);
            }
            "workspace.rename" => {
                self.workspace(required(value, "workspaceId")?)?.name =
                    required(value, "name")?.into()
            }
            "workspace.setRoot" => {
                self.workspace(required(value, "workspaceId")?)?
                    .root_directory = directory(required(value, "rootDirectory")?)?
            }
            "workspace.switch" => {
                let workspace_id = required(value, "workspaceId")?;
                self.workspace(workspace_id)?.last_opened_at = now();
                self.state.active_workspace_id = Some(workspace_id.into());
            }
            "workspace.close" => {
                let workspace_id = required(value, "workspaceId")?;
                self.workspace(workspace_id)?;
                self.state
                    .panes
                    .retain(|_, p| p.workspace_id != workspace_id);
                self.state.workspaces.retain(|w| w.id != workspace_id);
                if self.state.active_workspace_id.as_deref() == Some(workspace_id) {
                    self.state.active_workspace_id =
                        self.state.workspaces.first().map(|w| w.id.clone());
                }
            }
            "tab.create" | "pane.split" => {
                let (workspace_id, tab_id, target) = if operation == "tab.create" {
                    (required(value, "workspaceId")?.to_string(), id(), None)
                } else {
                    let pane_id = required(value, "paneId")?;
                    let pane = self
                        .state
                        .panes
                        .get(pane_id)
                        .ok_or_else(|| anyhow!("pane not found"))?;
                    (
                        pane.workspace_id.clone(),
                        pane.tab_id.clone(),
                        Some(pane_id.to_string()),
                    )
                };
                let mut pane = self.new_pane(&workspace_id, &tab_id, value)?;
                if let Some(target) = target {
                    self.tab(&tab_id)?.root_layout_node.split(
                        &target,
                        &pane.id,
                        required(value, "orientation")?,
                    )?;
                    self.tab(&tab_id)?.active_pane_id = pane.id.clone();
                } else {
                    let workspace = self.workspace(&workspace_id)?;
                    let title = shell_title(&pane.executable);
                    workspace.tabs.push(Tab {
                        id: tab_id.clone(),
                        workspace_id: workspace_id.clone(),
                        title,
                        title_mode: "automatic".into(),
                        root_layout_node: Layout::leaf(&pane.id),
                        active_pane_id: pane.id.clone(),
                        created_at: now(),
                        sort_order: workspace.tabs.len(),
                    });
                    workspace.active_tab_id = Some(tab_id);
                }
                let terminal = Terminal::launch(&mut pane, &self.pipe, None, self.history_limit())?;
                launched.push(pane.id.clone());
                self.terminals.insert(pane.id.clone(), terminal);
                self.state.panes.insert(pane.id.clone(), pane);
            }
            "tab.rename" => {
                let tab = self.tab(required(value, "tabId")?)?;
                tab.title = required(value, "title")?.into();
                tab.title_mode = "manual".into();
            }
            "tab.focus" => {
                let tab_id = required(value, "tabId")?;
                let workspace_id = self.tab(tab_id)?.workspace_id.clone();
                self.workspace(&workspace_id)?.active_tab_id = Some(tab_id.into());
            }
            "tab.close" => {
                for pane_id in self.tab(required(value, "tabId")?)?.root_layout_node.ids() {
                    self.close_pane(&pane_id)?;
                }
            }
            "pane.close" => self.close_pane(required(value, "paneId")?)?,
            "pane.focus" => {
                let pane_id = required(value, "paneId")?;
                let tab_id = self
                    .state
                    .panes
                    .get(pane_id)
                    .ok_or_else(|| anyhow!("pane not found"))?
                    .tab_id
                    .clone();
                self.tab(&tab_id)?.active_pane_id = pane_id.into();
            }
            "pane.resize" => {
                let path: Vec<usize> = serde_json::from_value(value["path"].clone())?;
                self.tab(required(value, "tabId")?)?
                    .root_layout_node
                    .resize(
                        &path,
                        value["ratio"]
                            .as_f64()
                            .ok_or_else(|| anyhow!("missing ratio"))?,
                    )?;
            }
            "settings.set" => {
                self.state
                    .settings
                    .insert(required(value, "key")?.into(), value["value"].clone());
            }
            _ => bail!("unknown operation: {operation}"),
        }
        Ok(())
    }
}

fn shell_title(executable: &str) -> String {
    let name = Path::new(executable)
        .file_stem()
        .and_then(|value| value.to_str())
        .unwrap_or(executable);
    match name.to_ascii_lowercase().as_str() {
        "pwsh" | "powershell" => "PowerShell".into(),
        "bash" => "Git Bash".into(),
        "cmd" => "Command Prompt".into(),
        _ => name.into(),
    }
}

fn valid_history_limit(value: usize) -> bool {
    matches!(value, 0 | 500 | 2_000 | 5_000 | 10_000 | 25_000)
}

impl Drop for Runtime {
    fn drop(&mut self) {
        logging::info("runtime.drop", "flushing persistence before shutdown");
        let _ = self.flush_persistence();
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn shell_titles_are_friendly() {
        assert_eq!(shell_title("pwsh.exe"), "PowerShell");
        assert_eq!(shell_title("powershell.exe"), "PowerShell");
        assert_eq!(
            shell_title(r"C:\Program Files\Git\usr\bin\bash.exe"),
            "Git Bash"
        );
        assert_eq!(shell_title("cmd.exe"), "Command Prompt");
    }
}
