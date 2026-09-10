#![cfg(windows)]

use anyhow::Result;
use base64::{engine::general_purpose::STANDARD, Engine};
use paneacea_runtime::ipc::{call, read_frame, write_frame};
use serde_json::json;
use std::{
    os::windows::process::CommandExt,
    path::Path,
    process::{Child, Command},
    time::Duration,
};
use tokio::{io::BufReader, net::windows::named_pipe::ClientOptions};

struct Daemon(Child);

impl Drop for Daemon {
    fn drop(&mut self) {
        let _ = self.0.kill();
        let _ = self.0.wait();
    }
}

async fn start(name: &str, directory: &Path) -> Result<Daemon> {
    let daemon = Daemon(
        Command::new(env!("CARGO_BIN_EXE_panacea-runtime"))
            .args(["--pipe", name, "--data"])
            .arg(directory)
            .creation_flags(0x08000000)
            .spawn()?,
    );
    for _ in 0..100 {
        if call(name, "state.get", json!({})).await.is_ok() {
            return Ok(daemon);
        }
        tokio::time::sleep(Duration::from_millis(100)).await;
    }
    anyhow::bail!("runtime did not start")
}

async fn snapshot(name: &str, pane_id: &str) -> Result<String> {
    let mut pipe = ClientOptions::new().open(format!(r"\\.\pipe\{name}"))?;
    write_frame(
        &mut pipe,
        &json!({"id":1,"method":"terminal.attach","params":{"paneId":pane_id}}),
    )
    .await?;
    let response = read_frame(&mut BufReader::new(pipe)).await?.unwrap();
    anyhow::ensure!(response["ok"] == true, "attach failed: {response}");
    Ok(
        String::from_utf8_lossy(&STANDARD.decode(response["result"]["data"].as_str().unwrap())?)
            .into_owned(),
    )
}

fn git_path(path: &Path) -> String {
    let value = path.to_string_lossy().replace('\\', "/");
    if value.len() >= 2 && value.as_bytes()[1] == b':' {
        format!("/{}{}", value[..1].to_ascii_lowercase(), &value[2..])
    } else {
        value
    }
}

#[tokio::test]
async fn git_bash_working_directory_is_persisted() -> Result<()> {
    let executable = Path::new(r"C:\Program Files\Git\usr\bin\bash.exe");
    if !executable.exists() {
        return Ok(());
    }
    let name = format!("paneacea-git-test-{}", uuid::Uuid::new_v4());
    let root = std::env::current_dir()?;
    let directory = root.join(".data").join(&name);
    std::fs::create_dir_all(&directory)?;
    let workspace_root = root.clone();
    let target_directory = workspace_root.join("src");
    let daemon = start(&name, &directory).await?;
    let state = call(
        &name,
        "workspace.create",
        json!({"name":"Git Bash", "rootDirectory":workspace_root}),
    )
    .await?;
    let workspace_id = state["activeWorkspaceId"].as_str().unwrap();
    call(
        &name,
        "settings.set",
        json!({
            "key":"defaultShell",
            "value":{
                "id":"git-bash",
                "executable":executable,
                "arguments":["--login","-i"]
            }
        }),
    )
    .await?;
    let state = call(&name, "tab.create", json!({"workspaceId":workspace_id})).await?;
    let pane_id = state["workspaces"][0]["tabs"][0]["activePaneId"]
        .as_str()
        .unwrap();
    tokio::time::sleep(Duration::from_millis(500)).await;
    call(
        &name,
        "pane.sendInput",
        json!({"paneId":pane_id,"data":format!("cd '{}'\r", git_path(&target_directory))}),
    )
    .await?;
    call(
        &name,
        "pane.sendInput",
        json!({"paneId":pane_id,"data":"declare -p PROMPT_COMMAND; printf 'PANEACEA-PWD:%s\\n' \"$PWD\"\r"}),
    )
    .await?;
    let mut output = String::new();
    for _ in 0..100 {
        output = snapshot(&name, pane_id).await?;
        if output.contains("PANEACEA-PWD:") {
            break;
        }
        tokio::time::sleep(Duration::from_millis(100)).await;
    }
    assert!(output.contains("PANEACEA-PWD:"), "Bash output: {output}");
    let expected = target_directory.to_string_lossy().replace('\\', "/");
    let mut current = String::new();
    for _ in 0..100 {
        let state = call(&name, "state.get", json!({})).await?;
        current = state["panes"][pane_id]["currentWorkingDirectory"]
            .as_str()
            .unwrap_or_default()
            .replace('\\', "/");
        if current.eq_ignore_ascii_case(&expected) {
            break;
        }
        tokio::time::sleep(Duration::from_millis(100)).await;
    }
    assert_eq!(
        current.to_ascii_lowercase(),
        expected.to_ascii_lowercase(),
        "Bash output: {output}"
    );
    tokio::time::sleep(Duration::from_millis(1200)).await;
    drop(daemon);

    let daemon = start(&name, &directory).await?;
    let state = call(&name, "state.get", json!({})).await?;
    let restored = state["panes"][pane_id]["currentWorkingDirectory"]
        .as_str()
        .unwrap_or_default()
        .replace('\\', "/");
    assert_eq!(restored.to_ascii_lowercase(), expected.to_ascii_lowercase());
    call(
        &name,
        "workspace.close",
        json!({"workspaceId":workspace_id}),
    )
    .await?;
    drop(daemon);
    Ok(())
}
