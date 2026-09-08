use crate::model::State;
use anyhow::Result;
use rusqlite::{params, Connection};

pub struct Store {
    connection: Connection,
}
impl Store {
    pub fn open(path: &std::path::Path) -> Result<Self> {
        let connection = Connection::open(path)?;
        connection.execute_batch("PRAGMA journal_mode=WAL; PRAGMA synchronous=FULL;
            CREATE TABLE IF NOT EXISTS settings (id TEXT PRIMARY KEY, payload TEXT NOT NULL);
            CREATE TABLE IF NOT EXISTS workspaces (id TEXT PRIMARY KEY, payload TEXT NOT NULL);
            CREATE TABLE IF NOT EXISTS tabs (id TEXT PRIMARY KEY, payload TEXT NOT NULL);
            CREATE TABLE IF NOT EXISTS panes (id TEXT PRIMARY KEY, payload TEXT NOT NULL);
            CREATE TABLE IF NOT EXISTS terminal_sessions (id TEXT PRIMARY KEY, payload TEXT NOT NULL);
            CREATE TABLE IF NOT EXISTS agent_sessions (id TEXT PRIMARY KEY, payload TEXT NOT NULL);")?;
        Ok(Self { connection })
    }
    pub fn load(&self) -> Result<State> {
        let mut statement = self
            .connection
            .prepare("SELECT payload FROM settings WHERE id='state'")?;
        let mut rows = statement.query([])?;
        match rows.next()? {
            Some(row) => Ok(serde_json::from_str(&row.get::<_, String>(0)?)?),
            None => Ok(State::default()),
        }
    }
    pub fn save(&mut self, state: &State) -> Result<()> {
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
        transaction.commit()?;
        Ok(())
    }
}
#[cfg(test)]
mod tests {
    use super::*;
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
}
