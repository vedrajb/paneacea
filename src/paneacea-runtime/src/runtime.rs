use crate::{model::*, persistence::Store, pty::Terminal};
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
        let store = Store::open(path)?;
        let state = store.load()?;
        let mut runtime = Self {
            state,
            terminals: BTreeMap::new(),
            store,
            pipe,
        };
        for pane in runtime.state.panes.values_mut() {
            pane.pid = None;
            if pane.title.trim().is_empty() {
                pane.title = shell_title(&pane.executable);
            }
            match Terminal::launch(pane, &runtime.pipe) {
                Ok(terminal) => {
                    runtime.terminals.insert(pane.id.clone(), terminal);
                }
                Err(error) => pane.error = Some(error.to_string()),
            }
        }
        runtime.store.save(&runtime.state)?;
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
    fn sync_titles(&mut self) {
        for (pane_id, terminal) in &self.terminals {
            let title = terminal.output.lock().unwrap().title.clone();
            if let Some(pane) = self.state.panes.get_mut(pane_id) {
                pane.title = title;
            }
        }
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
        self.sync_titles();
        match operation {
            "workspace.list" | "state.get" => return Ok(serde_json::to_value(&self.state)?),
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
                    .get_mut(required(&value, "paneId")?)
                    .ok_or_else(|| anyhow!("terminal unavailable"))?
                    .resize(rows, columns)?;
                return Ok(json!({}));
            }
            _ => {}
        }
        let previous = self.state.clone();
        let mut launched = Vec::new();
        let result = self.mutate(operation, &value, &mut launched).and_then(|_| {
            self.sync_titles();
            self.store.save(&self.state)
        });
        if let Err(error) = result {
            self.state = previous;
            for pane_id in launched {
                self.terminals.remove(&pane_id);
            }
            return Err(error);
        }
        self.terminals
            .retain(|pane_id, _| self.state.panes.contains_key(pane_id));
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
                let terminal = Terminal::launch(&mut pane, &self.pipe)?;
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
