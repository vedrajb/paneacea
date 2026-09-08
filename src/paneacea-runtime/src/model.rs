use anyhow::{bail, Result};
use serde::{Deserialize, Serialize};
use std::collections::BTreeMap;
use uuid::Uuid;

pub fn id() -> String {
    Uuid::new_v4().to_string()
}
pub fn now() -> u64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs()
}

pub const DEFAULT_TERMINAL_ROWS: u16 = 30;
pub const DEFAULT_TERMINAL_COLUMNS: u16 = 120;
pub const DEFAULT_TERMINAL_HISTORY_LINES: usize = 2000;
pub const TERMINAL_HISTORY_SETTING: &str = "terminalHistoryLines";

#[derive(Clone, Serialize, Deserialize, Debug, PartialEq)]
#[serde(tag = "type", rename_all = "camelCase")]
pub enum Layout {
    Pane {
        #[serde(rename = "paneId")]
        pane_id: String,
    },
    Split {
        orientation: String,
        ratio: f64,
        first: Box<Layout>,
        second: Box<Layout>,
    },
}
impl Layout {
    pub fn leaf(pane_id: &str) -> Self {
        Self::Pane {
            pane_id: pane_id.into(),
        }
    }
    pub fn ids(&self) -> Vec<String> {
        match self {
            Self::Pane { pane_id } => vec![pane_id.clone()],
            Self::Split { first, second, .. } => [first.ids(), second.ids()].concat(),
        }
    }
    pub fn split(&mut self, target: &str, pane_id: &str, orientation: &str) -> Result<()> {
        if !["vertical", "horizontal"].contains(&orientation) {
            bail!("invalid orientation");
        }
        match self {
            Self::Pane { pane_id: current } if current == target => {
                *self = Self::Split {
                    orientation: orientation.into(),
                    ratio: 0.5,
                    first: Box::new(self.clone()),
                    second: Box::new(Self::leaf(pane_id)),
                };
                Ok(())
            }
            Self::Split { first, second, .. } => {
                if first.ids().iter().any(|p| p == target) {
                    first.split(target, pane_id, orientation)
                } else {
                    second.split(target, pane_id, orientation)
                }
            }
            _ => bail!("pane not in layout"),
        }
    }
    pub fn remove(self, target: &str) -> Option<Self> {
        match self {
            Self::Pane { ref pane_id } => {
                if pane_id == target {
                    None
                } else {
                    Some(self)
                }
            }
            Self::Split {
                orientation,
                ratio,
                first,
                second,
            } => match (first.remove(target), second.remove(target)) {
                (Some(first), Some(second)) => Some(Self::Split {
                    orientation,
                    ratio,
                    first: Box::new(first),
                    second: Box::new(second),
                }),
                (first, second) => first.or(second),
            },
        }
    }
    pub fn resize(&mut self, path: &[usize], ratio: f64) -> Result<()> {
        if !ratio.is_finite() || !(0.1..=0.9).contains(&ratio) {
            bail!("ratio must be between 0.1 and 0.9");
        }
        match self {
            Self::Split {
                ratio: current,
                first,
                second,
                ..
            } => {
                if path.is_empty() {
                    *current = ratio;
                    Ok(())
                } else {
                    match path[0] {
                        0 => first.resize(&path[1..], ratio),
                        1 => second.resize(&path[1..], ratio),
                        _ => bail!("invalid layout path"),
                    }
                }
            }
            _ => bail!("layout path does not refer to split"),
        }
    }
}

#[derive(Clone, Serialize, Deserialize, Debug)]
#[serde(rename_all = "camelCase")]
pub struct Workspace {
    pub id: String,
    pub name: String,
    pub root_directory: String,
    pub tabs: Vec<Tab>,
    pub active_tab_id: Option<String>,
    pub created_at: u64,
    pub last_opened_at: u64,
}
#[derive(Clone, Serialize, Deserialize, Debug)]
#[serde(rename_all = "camelCase")]
pub struct Tab {
    pub id: String,
    pub workspace_id: String,
    pub title: String,
    pub title_mode: String,
    pub root_layout_node: Layout,
    pub active_pane_id: String,
    pub created_at: u64,
    pub sort_order: usize,
}
#[derive(Clone, Serialize, Deserialize, Debug)]
#[serde(rename_all = "camelCase")]
pub struct Pane {
    pub id: String,
    pub workspace_id: String,
    pub tab_id: String,
    pub profile_id: String,
    pub executable: String,
    #[serde(default)]
    pub title: String,
    pub arguments: Vec<String>,
    pub environment: BTreeMap<String, String>,
    pub initial_working_directory: String,
    pub current_working_directory: String,
    #[serde(default = "default_terminal_rows")]
    pub terminal_rows: u16,
    #[serde(default = "default_terminal_columns")]
    pub terminal_columns: u16,
    pub runtime_terminal_id: String,
    pub pid: Option<u32>,
    pub generation: String,
    pub agent: Option<serde_json::Value>,
    pub error: Option<String>,
}

fn default_terminal_rows() -> u16 {
    DEFAULT_TERMINAL_ROWS
}

fn default_terminal_columns() -> u16 {
    DEFAULT_TERMINAL_COLUMNS
}
#[derive(Clone, Serialize, Deserialize, Debug, Default)]
#[serde(rename_all = "camelCase")]
pub struct State {
    pub workspaces: Vec<Workspace>,
    pub panes: BTreeMap<String, Pane>,
    pub active_workspace_id: Option<String>,
    pub settings: BTreeMap<String, serde_json::Value>,
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn split_close_preserves_sibling_tree() {
        let mut layout = Layout::leaf("a");
        layout.split("a", "b", "vertical").unwrap();
        layout.split("b", "c", "horizontal").unwrap();
        layout.resize(&[1], 0.7).unwrap();
        let restored: Layout =
            serde_json::from_str(&serde_json::to_string(&layout).unwrap()).unwrap();
        assert_eq!(layout, restored);
        assert_eq!(layout.remove("b").unwrap().ids(), vec!["a", "c"]);
    }
    #[test]
    fn rejects_invalid_layout_edits() {
        let mut layout = Layout::leaf("a");
        assert!(layout.split("missing", "b", "vertical").is_err());
        assert!(layout.split("a", "b", "diagonal").is_err());
        layout.split("a", "b", "vertical").unwrap();
        assert!(layout.resize(&[], f64::NAN).is_err());
        assert!(layout.resize(&[2], 0.5).is_err());
        assert!(layout.resize(&[], 1.0).is_err());
    }
}
