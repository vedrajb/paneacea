#![cfg(windows)]
use anyhow::Result;
use base64::{engine::general_purpose::STANDARD, Engine};
use paneacea_runtime::ipc::{call, read_frame, write_frame};
use serde_json::{json, Value};
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
async fn attach(name: &str, pane_id: &str) -> Result<Value> {
    let mut pipe = ClientOptions::new().open(format!(r"\\.\pipe\{name}"))?;
    write_frame(
        &mut pipe,
        &json!({"id":1,"method":"terminal.attach","params":{"paneId":pane_id}}),
    )
    .await?;
    let response = read_frame(&mut BufReader::new(pipe)).await?.unwrap();
    anyhow::ensure!(response["ok"] == true, "attach failed: {response}");
    Ok(response["result"].clone())
}
async fn snapshot(name: &str, pane_id: &str) -> Result<String> {
    let result = attach(name, pane_id).await?;
    Ok(String::from_utf8_lossy(&STANDARD.decode(result["data"].as_str().unwrap())?).into_owned())
}

#[tokio::test]
async fn conpty_detach_layout_restart_and_validation() -> Result<()> {
    let name = format!("paneacea-test-{}", uuid::Uuid::new_v4());
    let root = std::env::current_dir()?;
    let directory = root.join(".data").join(&name);
    std::fs::create_dir_all(&directory)?;
    let first_directory = directory.join("first");
    let second_directory = directory.join("second");
    let third_directory = directory.join("third");
    std::fs::create_dir_all(&first_directory)?;
    std::fs::create_dir_all(&second_directory)?;
    std::fs::create_dir_all(&third_directory)?;
    let daemon = start(&name, &directory).await?;
    let state = call(
        &name,
        "workspace.create",
        json!({"name":"Integration","rootDirectory":root}),
    )
    .await?;
    let workspace_id = state["activeWorkspaceId"].as_str().unwrap();
    call(
        &name,
        "settings.set",
        json!({"key":"defaultShell","value":{"id":"pwsh","executable":"powershell.exe","arguments":["-NoLogo","-NoProfile"]}}),
    )
    .await?;
    call(
        &name,
        "settings.set",
        json!({"key":"terminalHistoryLines","value":500}),
    )
    .await?;
    let state = call(&name, "tab.create", json!({"workspaceId":workspace_id})).await?;
    let tab_id = state["workspaces"][0]["activeTabId"].as_str().unwrap();
    let pane_id = state["workspaces"][0]["tabs"][0]["activePaneId"]
        .as_str()
        .unwrap();
    assert_eq!(state["panes"][pane_id]["executable"], "powershell.exe");
    assert_eq!(
        state["panes"][pane_id]["arguments"],
        json!(["-NoLogo", "-NoProfile"])
    );
    assert_eq!(state["workspaces"][0]["tabs"][0]["title"], "PowerShell");
    let pid = state["panes"][pane_id]["pid"].clone();
    let generation = state["panes"][pane_id]["generation"].clone();
    assert!(pid.as_u64().unwrap() > 0);
    assert_eq!(
        state["panes"][pane_id]["initialWorkingDirectory"],
        root.to_str().unwrap()
    );
    let first_command = format!(
        "Set-Location -LiteralPath '{}'; 1..50 | ForEach-Object {{ Write-Output ('ONE-' + $_) }}; Write-Output 'PERSIST-PANE-ONE'\r",
        first_directory.to_string_lossy().replace('\'', "''")
    );
    call(
        &name,
        "pane.sendInput",
        json!({"paneId":pane_id,"data":first_command}),
    )
    .await?;
    let mut first_saved = false;
    let mut first_cwd = String::new();
    let mut first_output = String::new();
    for _ in 0..100 {
        let current = call(&name, "state.get", json!({})).await?;
        let cwd = current["panes"][pane_id]["currentWorkingDirectory"]
            .as_str()
            .unwrap_or_default();
        first_cwd = cwd.into();
        first_output = snapshot(&name, pane_id).await?;
        if first_cwd.replace('\\', "/") == first_directory.to_string_lossy().replace('\\', "/")
            && first_output.contains("PERSIST-PANE-ONE")
        {
            first_saved = true;
            break;
        }
        tokio::time::sleep(Duration::from_millis(100)).await;
    }
    assert!(
        first_saved,
        "first pane should persist its working directory and output; cwd={first_cwd}; output={first_output}"
    );
    call(
        &name,
        "settings.set",
        json!({"key":"defaultShell","value":{"id":"cmd","executable":"cmd.exe","arguments":[]}}),
    )
    .await?;
    let cmd_state = call(&name, "tab.create", json!({"workspaceId":workspace_id})).await?;
    let cmd_tab_id = cmd_state["workspaces"][0]["activeTabId"].as_str().unwrap();
    let cmd_pane_id = cmd_state["workspaces"][0]["tabs"]
        .as_array()
        .unwrap()
        .iter()
        .find(|tab| tab["id"] == cmd_tab_id)
        .unwrap()["activePaneId"]
        .as_str()
        .unwrap();
    assert_eq!(cmd_state["panes"][cmd_pane_id]["executable"], "cmd.exe");
    assert_eq!(cmd_state["panes"][cmd_pane_id]["arguments"], json!([]));
    call(&name, "tab.close", json!({"tabId":cmd_tab_id})).await?;
    call(
        &name,
        "settings.set",
        json!({"key":"defaultShell","value":{"id":"pwsh","executable":"powershell.exe","arguments":["-NoLogo","-NoProfile"]}}),
    )
    .await?;
    snapshot(&name, pane_id).await?;
    call(&name, "pane.sendInput", json!({"paneId":pane_id,"data":"$tinkerTest = 'LIVE'; Write-Output ($tinkerTest + '-DETACHED-OK')\r"})).await?;
    let mut found = false;
    for _ in 0..100 {
        if snapshot(&name, pane_id).await?.contains("LIVE-DETACHED-OK") {
            found = true;
            break;
        }
        tokio::time::sleep(Duration::from_millis(100)).await;
    }
    assert!(found, "PowerShell output should survive client detach");
    let same = call(&name, "state.get", json!({})).await?;
    assert_eq!(same["panes"][pane_id]["pid"], pid);
    call(
        &name,
        "terminal.resize",
        json!({"paneId":pane_id,"rows":40,"columns":100}),
    )
    .await?;
    assert!(call(
        &name,
        "terminal.resize",
        json!({"paneId":pane_id,"rows":0,"columns":100})
    )
    .await
    .is_err());
    assert!(call(
        &name,
        "pane.split",
        json!({"paneId":pane_id,"orientation":"diagonal"})
    )
    .await
    .is_err());
    assert_eq!(
        call(&name, "state.get", json!({})).await?["panes"]
            .as_object()
            .unwrap()
            .len(),
        1
    );
    let state = call(
        &name,
        "pane.split",
        json!({"paneId":pane_id,"orientation":"vertical"}),
    )
    .await?;
    let second = state["workspaces"][0]["tabs"][0]["activePaneId"]
        .as_str()
        .unwrap();
    for _ in 0..100 {
        if snapshot(&name, second).await?.contains("PS ") {
            break;
        }
        tokio::time::sleep(Duration::from_millis(100)).await;
    }
    call(
        &name,
        "pane.sendInput",
        json!({"paneId":second,"data":format!("Set-Location -LiteralPath '{}'; 1..50 | ForEach-Object {{ Write-Output ('TWO-' + $_) }}; Write-Output 'PERSIST-PANE-TWO'\r", second_directory.to_string_lossy().replace('\'', "''"))}),
    )
    .await?;
    call(
        &name,
        "pane.split",
        json!({"paneId":second,"orientation":"horizontal"}),
    )
    .await?;
    let split_state = call(&name, "state.get", json!({})).await?;
    let third = split_state["workspaces"][0]["tabs"][0]["activePaneId"]
        .as_str()
        .unwrap();
    for _ in 0..100 {
        if snapshot(&name, third).await?.contains("PS ") {
            break;
        }
        tokio::time::sleep(Duration::from_millis(100)).await;
    }
    call(
        &name,
        "pane.sendInput",
        json!({"paneId":third,"data":format!("Set-Location -LiteralPath '{}'; Write-Output 'PERSIST-PANE-THREE'\r", third_directory.to_string_lossy().replace('\'', "''"))}),
    )
    .await?;
    call(
        &name,
        "pane.resize",
        json!({"tabId":tab_id,"path":[1],"ratio":0.7}),
    )
    .await?;
    call(
        &name,
        "tab.rename",
        json!({"tabId":tab_id,"title":"Saved layout"}),
    )
    .await?;
    for _ in 0..100 {
        if snapshot(&name, second).await?.contains("PERSIST-PANE-TWO") {
            break;
        }
        tokio::time::sleep(Duration::from_millis(100)).await;
    }
    let viewport = call(
        &name,
        "terminal.viewport.set",
        json!({"paneId":second,"offset":2}),
    )
    .await?;
    assert!(viewport["viewportOffset"].as_u64().unwrap() > 0);
    tokio::time::sleep(Duration::from_millis(1200)).await;
    let before = call(&name, "state.get", json!({})).await?;
    drop(daemon);
    let daemon = start(&name, &directory).await?;
    let restored = call(&name, "state.get", json!({})).await?;
    assert_eq!(restored["workspaces"], before["workspaces"]);
    assert_ne!(restored["panes"][pane_id]["generation"], generation);
    assert_eq!(
        restored["panes"][pane_id]["arguments"],
        json!(["-NoLogo", "-NoProfile"])
    );
    assert!(restored["panes"][pane_id]["error"].is_null());
    assert_eq!(
        restored["panes"][pane_id]["currentWorkingDirectory"]
            .as_str()
            .unwrap()
            .replace('\\', "/"),
        first_directory.to_string_lossy().replace('\\', "/")
    );
    for pane in [pane_id, second, third] {
        let restored_snapshot = attach(&name, pane).await?;
        let restored_output =
            String::from_utf8_lossy(&STANDARD.decode(restored_snapshot["data"].as_str().unwrap())?)
                .into_owned();
        assert!(restored_snapshot["restored"] == true);
        let marker = if pane == pane_id {
            "PERSIST-PANE-ONE"
        } else if pane == second {
            "PERSIST-PANE-TWO"
        } else {
            "PERSIST-PANE-THREE"
        };
        assert!(restored_output.contains(marker));
        if pane == pane_id {
            assert!(restored_output.contains("Paneacea restored terminal history"));
        }
    }
    let restored_second = attach(&name, second).await?;
    assert_eq!(restored_second["viewportOffset"].as_u64(), Some(0));
    let closed = call(&name, "pane.close", json!({"paneId":second})).await?;
    assert_eq!(closed["panes"].as_object().unwrap().len(), 2);
    let closed: Value = call(
        &name,
        "workspace.close",
        json!({"workspaceId":workspace_id}),
    )
    .await?;
    assert!(closed["panes"].as_object().unwrap().is_empty());
    assert!(closed["workspaces"].as_array().unwrap().is_empty());
    drop(daemon);
    Ok(())
}
