use std::{
    fs::{create_dir_all, File, OpenOptions},
    io::Write,
    path::{Path, PathBuf},
    sync::{Mutex, OnceLock},
    time::{SystemTime, UNIX_EPOCH},
};

static LOG_FILE: OnceLock<Mutex<File>> = OnceLock::new();
static LOG_PATH: OnceLock<PathBuf> = OnceLock::new();

pub fn init(path: &Path) -> std::io::Result<()> {
    if LOG_FILE.get().is_some() {
        return Ok(());
    }
    if let Some(parent) = path.parent() {
        create_dir_all(parent)?;
    }
    let file = OpenOptions::new().create(true).append(true).open(path)?;
    let _ = LOG_PATH.set(path.to_path_buf());
    let _ = LOG_FILE.set(Mutex::new(file));
    event("logging.initialized", &format!("path={}", path.display()));
    Ok(())
}

pub fn path() -> Option<PathBuf> {
    LOG_PATH.get().cloned()
}

pub fn info(event_name: &str, details: impl AsRef<str>) {
    write_line("INFO", event_name, details.as_ref());
}

pub fn error(event_name: &str, details: impl AsRef<str>) {
    write_line("ERROR", event_name, details.as_ref());
}

fn event(event_name: &str, details: &str) {
    info(event_name, details);
}

fn write_line(level: &str, event_name: &str, details: &str) {
    let timestamp = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|duration| duration.as_millis())
        .unwrap_or_default();
    let details = details.replace(['\r', '\n'], " ");
    let line = format!("{timestamp} [{level}] {event_name} {details}");
    eprintln!("{line}");
    if let Some(file) = LOG_FILE.get() {
        if let Ok(mut file) = file.lock() {
            let _ = writeln!(file, "{line}");
            let _ = file.flush();
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn log_path_is_available_before_initialization() {
        assert!(path().is_none());
    }
}
