use crate::{logging, model::State};
use anyhow::{Context, Result};
use rusqlite::{params, Connection};
use std::path::Path;

const HISTORY_FORMAT_VERSION: i64 = 2;

#[derive(Clone, Debug, PartialEq)]
pub struct SavedHistory {
    pub data: Vec<u8>,
    pub rows: u16,
    pub columns: u16,
}

pub struct Store {
    connection: Connection,
}
impl Store {
    pub fn open(path: &Path) -> Result<Self> {
        logging::info("store.open", &format!("database={}", path.display()));
        let mut connection = Connection::open(path)?;
        connection.execute_batch("PRAGMA journal_mode=WAL; PRAGMA synchronous=FULL;
            CREATE TABLE IF NOT EXISTS settings (id TEXT PRIMARY KEY, payload TEXT NOT NULL);
            CREATE TABLE IF NOT EXISTS workspaces (id TEXT PRIMARY KEY, payload TEXT NOT NULL);
            CREATE TABLE IF NOT EXISTS tabs (id TEXT PRIMARY KEY, payload TEXT NOT NULL);
            CREATE TABLE IF NOT EXISTS panes (id TEXT PRIMARY KEY, payload TEXT NOT NULL);
            CREATE TABLE IF NOT EXISTS terminal_sessions (id TEXT PRIMARY KEY, payload TEXT NOT NULL);
            CREATE TABLE IF NOT EXISTS agent_sessions (id TEXT PRIMARY KEY, payload TEXT NOT NULL);
            CREATE TABLE IF NOT EXISTS terminal_history (
                pane_id TEXT PRIMARY KEY,
                payload BLOB NOT NULL,
                rows INTEGER NOT NULL,
                columns INTEGER NOT NULL,
                updated_at INTEGER NOT NULL,
                format_version INTEGER NOT NULL
            );")?;
        migrate_history_table(&mut connection)?;
        logging::info("store.ready", &format!("database={}", path.display()));
        Ok(Self { connection })
    }
    pub fn load(&self) -> Result<State> {
        let mut statement = self
            .connection
            .prepare("SELECT payload FROM settings WHERE id='state'")?;
        let mut rows = statement.query([])?;
        let state = match rows.next()? {
            Some(row) => serde_json::from_str(&row.get::<_, String>(0)?)?,
            None => State::default(),
        };
        logging::info(
            "state.load",
            &format!(
                "workspaces={} tabs={} panes={} settings={} active_workspace={}",
                state.workspaces.len(),
                state
                    .workspaces
                    .iter()
                    .map(|workspace| workspace.tabs.len())
                    .sum::<usize>(),
                state.panes.len(),
                state.settings.len(),
                state.active_workspace_id.as_deref().unwrap_or("none")
            ),
        );
        Ok(state)
    }
    pub fn save(&mut self, state: &State) -> Result<()> {
        logging::info(
            "state.save.start",
            &format!(
                "workspaces={} tabs={} panes={} settings={}",
                state.workspaces.len(),
                state
                    .workspaces
                    .iter()
                    .map(|workspace| workspace.tabs.len())
                    .sum::<usize>(),
                state.panes.len(),
                state.settings.len()
            ),
        );
        let transaction = self.connection.transaction()?;
        transaction.execute(
            "INSERT OR REPLACE INTO settings VALUES ('state', ?1)",
            [serde_json::to_string(state)?],
        )?;
        for table in [
            "workspaces",
            "tabs",
            "panes",
            "terminal_sessions",
            "agent_sessions",
        ] {
            transaction.execute(&format!("DELETE FROM {table}"), [])?;
        }
        for workspace in &state.workspaces {
            transaction.execute(
                "INSERT INTO workspaces VALUES (?1, ?2)",
                params![workspace.id, serde_json::to_string(workspace)?],
            )?;
            for tab in &workspace.tabs {
                transaction.execute(
                    "INSERT INTO tabs VALUES (?1, ?2)",
                    params![tab.id, serde_json::to_string(tab)?],
                )?;
            }
        }
        for pane in state.panes.values() {
            transaction.execute(
                "INSERT INTO panes VALUES (?1, ?2)",
                params![pane.id, serde_json::to_string(pane)?],
            )?;
            transaction.execute("INSERT INTO terminal_sessions VALUES (?1, ?2)", params![pane.runtime_terminal_id, serde_json::to_string(&serde_json::json!({"paneId":pane.id,"pid":pane.pid,"generation":pane.generation}))?])?;
            if let Some(agent) = &pane.agent {
                transaction.execute(
                    "INSERT INTO agent_sessions VALUES (?1, ?2)",
                    params![pane.id, serde_json::to_string(agent)?],
                )?;
            }
        }
        let removed_history = transaction.execute(
            "DELETE FROM terminal_history WHERE pane_id NOT IN (SELECT id FROM panes)",
            [],
        )?;
        transaction.commit()?;
        logging::info(
            "state.save.complete",
            &format!(
                "panes={} removed_history_rows={removed_history}",
                state.panes.len()
            ),
        );
        Ok(())
    }

    pub fn load_history(&self, pane_id: &str) -> Result<Option<SavedHistory>> {
        let mut statement = self.connection.prepare(
            "SELECT payload, rows, columns, format_version
             FROM terminal_history WHERE pane_id=?1",
        )?;
        let mut rows = statement.query([pane_id])?;
        let Some(row) = rows.next()? else {
            logging::info("history.load.missing", &format!("pane_id={pane_id}"));
            return Ok(None);
        };
        let payload: Vec<u8> = row.get(0)?;
        let format_version: i64 = row.get(3)?;
        if format_version != HISTORY_FORMAT_VERSION {
            logging::error(
                "history.load.invalid_format",
                &format!("pane_id={pane_id} format_version={format_version}"),
            );
            return Ok(None);
        }
        let data = match unprotect(&payload, pane_id) {
            Ok(data) => data,
            Err(error) => {
                logging::error(
                    "history.load.decrypt_failed",
                    &format!(
                        "pane_id={pane_id} payload_bytes={} error={error:#}",
                        payload.len()
                    ),
                );
                eprintln!("could not restore terminal history for pane {pane_id}: {error:#}");
                return Ok(None);
            }
        };
        let rows = match u16::try_from(row.get::<_, i64>(1)?) {
            Ok(value) if (1..=500).contains(&value) => value,
            value => {
                logging::error(
                    "history.load.invalid_rows",
                    &format!("pane_id={pane_id} value={value:?}"),
                );
                return Ok(None);
            }
        };
        let columns = match u16::try_from(row.get::<_, i64>(2)?) {
            Ok(value) if (1..=1000).contains(&value) => value,
            value => {
                logging::error(
                    "history.load.invalid_columns",
                    &format!("pane_id={pane_id} value={value:?}"),
                );
                return Ok(None);
            }
        };
        logging::info(
            "history.load.complete",
            &format!(
                "pane_id={pane_id} payload_bytes={} restored_bytes={} rows={rows} columns={columns}",
                payload.len(),
                data.len()
            ),
        );
        Ok(Some(SavedHistory {
            data,
            rows,
            columns,
        }))
    }

    pub fn save_history(&mut self, pane_id: &str, history: &SavedHistory) -> Result<()> {
        let payload = protect(&history.data, pane_id)?;
        let rows = i64::from(history.rows);
        let columns = i64::from(history.columns);
        self.connection.execute(
            "INSERT OR REPLACE INTO terminal_history
             (pane_id, payload, rows, columns, updated_at, format_version)
             VALUES (?1, ?2, ?3, ?4, ?5, ?6)",
            params![
                pane_id,
                payload,
                rows,
                columns,
                crate::model::now(),
                HISTORY_FORMAT_VERSION
            ],
        )?;
        logging::info(
            "history.save.complete",
            &format!(
                "pane_id={pane_id} data_bytes={} payload_bytes={} rows={} columns={}",
                history.data.len(),
                payload.len(),
                history.rows,
                history.columns
            ),
        );
        Ok(())
    }

    pub fn clear_history(&mut self) -> Result<()> {
        let removed = self
            .connection
            .execute("DELETE FROM terminal_history", [])?;
        logging::info("history.clear", &format!("removed_rows={removed}"));
        Ok(())
    }
}

fn migrate_history_table(connection: &mut Connection) -> Result<()> {
    let columns = {
        let mut statement = connection.prepare("PRAGMA table_info(terminal_history)")?;
        let columns = statement
            .query_map([], |row| row.get::<_, String>(1))?
            .collect::<rusqlite::Result<Vec<_>>>()?;
        columns
    };
    if !columns.iter().any(|column| column == "viewport_offset") {
        return Ok(());
    }
    let transaction = connection.transaction()?;
    transaction.execute_batch(
        "CREATE TABLE terminal_history_without_viewport (
            pane_id TEXT PRIMARY KEY,
            payload BLOB NOT NULL,
            rows INTEGER NOT NULL,
            columns INTEGER NOT NULL,
            updated_at INTEGER NOT NULL,
            format_version INTEGER NOT NULL
        );
        INSERT INTO terminal_history_without_viewport
            (pane_id, payload, rows, columns, updated_at, format_version)
            SELECT pane_id, payload, rows, columns, updated_at, format_version
            FROM terminal_history;
        DROP TABLE terminal_history;
        ALTER TABLE terminal_history_without_viewport RENAME TO terminal_history;",
    )?;
    transaction.commit()?;
    logging::info(
        "history.schema.migrated",
        "removed persisted viewport offsets",
    );
    Ok(())
}

fn protect(data: &[u8], pane_id: &str) -> Result<Vec<u8>> {
    #[cfg(windows)]
    {
        use windows_sys::Win32::{
            Foundation::LocalFree,
            Security::Cryptography::{
                CryptProtectData, CRYPTPROTECT_UI_FORBIDDEN, CRYPT_INTEGER_BLOB,
            },
        };
        let entropy_bytes = format!("Paneacea terminal history v1:{pane_id}").into_bytes();
        let input = CRYPT_INTEGER_BLOB {
            cbData: u32::try_from(data.len()).context("terminal history is too large")?,
            pbData: data.as_ptr() as *mut u8,
        };
        let entropy = CRYPT_INTEGER_BLOB {
            cbData: u32::try_from(entropy_bytes.len()).context("history entropy is too large")?,
            pbData: entropy_bytes.as_ptr() as *mut u8,
        };
        let mut output = CRYPT_INTEGER_BLOB {
            cbData: 0,
            pbData: std::ptr::null_mut(),
        };
        let protected = unsafe {
            CryptProtectData(
                &input,
                std::ptr::null(),
                &entropy,
                std::ptr::null(),
                std::ptr::null(),
                CRYPTPROTECT_UI_FORBIDDEN,
                &mut output,
            )
        };
        if protected == 0 {
            return Err(std::io::Error::last_os_error()).context("DPAPI encryption failed");
        }
        let result =
            unsafe { std::slice::from_raw_parts(output.pbData, output.cbData as usize).to_vec() };
        unsafe { LocalFree(output.pbData as _) };
        Ok(result)
    }
    #[cfg(not(windows))]
    {
        let _ = pane_id;
        Ok(data.to_vec())
    }
}

fn unprotect(data: &[u8], pane_id: &str) -> Result<Vec<u8>> {
    #[cfg(windows)]
    {
        use windows_sys::Win32::{
            Foundation::LocalFree,
            Security::Cryptography::{
                CryptUnprotectData, CRYPTPROTECT_UI_FORBIDDEN, CRYPT_INTEGER_BLOB,
            },
        };
        let entropy_bytes = format!("Paneacea terminal history v1:{pane_id}").into_bytes();
        let input = CRYPT_INTEGER_BLOB {
            cbData: u32::try_from(data.len()).context("encrypted history is too large")?,
            pbData: data.as_ptr() as *mut u8,
        };
        let entropy = CRYPT_INTEGER_BLOB {
            cbData: u32::try_from(entropy_bytes.len()).context("history entropy is too large")?,
            pbData: entropy_bytes.as_ptr() as *mut u8,
        };
        let mut output = CRYPT_INTEGER_BLOB {
            cbData: 0,
            pbData: std::ptr::null_mut(),
        };
        let unprotected = unsafe {
            CryptUnprotectData(
                &input,
                std::ptr::null_mut(),
                &entropy,
                std::ptr::null(),
                std::ptr::null(),
                CRYPTPROTECT_UI_FORBIDDEN,
                &mut output,
            )
        };
        if unprotected == 0 {
            return Err(std::io::Error::last_os_error()).context("DPAPI decryption failed");
        }
        let result =
            unsafe { std::slice::from_raw_parts(output.pbData, output.cbData as usize).to_vec() };
        unsafe { LocalFree(output.pbData as _) };
        Ok(result)
    }
    #[cfg(not(windows))]
    {
        let _ = pane_id;
        Ok(data.to_vec())
    }
}
#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn history_schema_migration_removes_viewport_column() {
        let mut connection = Connection::open(":memory:").unwrap();
        connection
            .execute_batch(
                "CREATE TABLE terminal_history (
                    pane_id TEXT PRIMARY KEY,
                    payload BLOB NOT NULL,
                    rows INTEGER NOT NULL,
                    columns INTEGER NOT NULL,
                    viewport_offset INTEGER NOT NULL,
                    updated_at INTEGER NOT NULL,
                    format_version INTEGER NOT NULL
                );
                INSERT INTO terminal_history
                    (pane_id, payload, rows, columns, viewport_offset, updated_at, format_version)
                    VALUES ('pane', X'01', 30, 120, 4, 1, 1);",
            )
            .unwrap();
        migrate_history_table(&mut connection).unwrap();
        let columns = connection
            .prepare("PRAGMA table_info(terminal_history)")
            .unwrap()
            .query_map([], |row| row.get::<_, String>(1))
            .unwrap()
            .collect::<rusqlite::Result<Vec<_>>>()
            .unwrap();
        assert!(!columns.iter().any(|column| column == "viewport_offset"));
        let payload: Vec<u8> = connection
            .query_row(
                "SELECT payload FROM terminal_history WHERE pane_id='pane'",
                [],
                |row| row.get(0),
            )
            .unwrap();
        assert_eq!(payload, vec![1]);
    }

    #[test]
    fn sqlite_round_trip() {
        let mut store = Store::open(std::path::Path::new(":memory:")).unwrap();
        let mut state = State::default();
        state.settings.insert("fontSize".into(), 16.into());
        store.save(&state).unwrap();
        assert_eq!(store.load().unwrap().settings["fontSize"], 16);
        state.settings.clear();
        store.save(&state).unwrap();
        assert!(store.load().unwrap().settings.is_empty());
    }

    #[test]
    fn history_round_trip_survives_state_save() {
        let mut store = Store::open(Path::new(":memory:")).unwrap();
        let mut state = State::default();
        state.panes.insert(
            "pane".into(),
            crate::model::Pane {
                id: "pane".into(),
                workspace_id: "workspace".into(),
                tab_id: "tab".into(),
                profile_id: "default".into(),
                executable: "shell".into(),
                title: "shell".into(),
                arguments: vec![],
                environment: Default::default(),
                initial_working_directory: ".".into(),
                current_working_directory: ".".into(),
                terminal_rows: 30,
                terminal_columns: 120,
                runtime_terminal_id: "runtime".into(),
                pid: None,
                generation: "generation".into(),
                agent: None,
                error: None,
            },
        );
        store.save(&state).unwrap();
        let history = SavedHistory {
            data: b"history".to_vec(),
            rows: 30,
            columns: 120,
        };
        store.save_history("pane", &history).unwrap();
        store.save(&state).unwrap();
        assert_eq!(store.load_history("pane").unwrap(), Some(history));
    }

    #[test]
    fn state_save_removes_history_for_deleted_panes() {
        let mut store = Store::open(Path::new(":memory:")).unwrap();
        let mut state = State::default();
        state.panes.insert(
            "pane".into(),
            crate::model::Pane {
                id: "pane".into(),
                workspace_id: "workspace".into(),
                tab_id: "tab".into(),
                profile_id: "default".into(),
                executable: "shell".into(),
                title: "shell".into(),
                arguments: vec![],
                environment: Default::default(),
                initial_working_directory: ".".into(),
                current_working_directory: ".".into(),
                terminal_rows: 30,
                terminal_columns: 120,
                runtime_terminal_id: "runtime".into(),
                pid: None,
                generation: "generation".into(),
                agent: None,
                error: None,
            },
        );
        store.save(&state).unwrap();
        store
            .save_history(
                "pane",
                &SavedHistory {
                    data: vec![1],
                    rows: 30,
                    columns: 120,
                },
            )
            .unwrap();
        state.panes.clear();
        store.save(&state).unwrap();
        assert!(store.load_history("pane").unwrap().is_none());
    }
}
