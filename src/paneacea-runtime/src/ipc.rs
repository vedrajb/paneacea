use crate::runtime::Runtime;
use anyhow::{anyhow, Result};
use base64::{engine::general_purpose::STANDARD, Engine};
use serde_json::{json, Value};
use std::{
    ffi::c_void,
    sync::{Arc, Mutex},
};
use tokio::{
    io::{AsyncBufReadExt, AsyncRead, AsyncWrite, AsyncWriteExt, BufReader},
    net::windows::named_pipe::{ClientOptions, NamedPipeServer, ServerOptions},
};
use windows_sys::Win32::{
    Foundation::LocalFree,
    Security::{
        Authorization::ConvertStringSecurityDescriptorToSecurityDescriptorW, SECURITY_ATTRIBUTES,
    },
};

pub fn default_pipe() -> String {
    let user = std::env::var("USERNAME").unwrap_or_else(|_| "default".into());
    format!(
        "paneacea-{}",
        user.chars()
            .map(|c| if c.is_ascii_alphanumeric() { c } else { '_' })
            .collect::<String>()
    )
}
fn server(name: &str, first: bool) -> Result<NamedPipeServer> {
    let descriptor: Vec<u16> = "D:P(A;;GA;;;OW)\0".encode_utf16().collect();
    let mut security = std::ptr::null_mut();
    unsafe {
        if ConvertStringSecurityDescriptorToSecurityDescriptorW(
            descriptor.as_ptr(),
            1,
            &mut security,
            std::ptr::null_mut(),
        ) == 0
        {
            return Err(std::io::Error::last_os_error().into());
        }
        let attributes = SECURITY_ATTRIBUTES {
            nLength: std::mem::size_of::<SECURITY_ATTRIBUTES>() as u32,
            lpSecurityDescriptor: security,
            bInheritHandle: 0,
        };
        let result = ServerOptions::new()
            .first_pipe_instance(first)
            .reject_remote_clients(true)
            .create_with_security_attributes_raw(
                format!(r"\\.\pipe\{name}"),
                &attributes as *const _ as *mut c_void,
            );
        LocalFree(security);
        Ok(result?)
    }
}
pub async fn read_frame<R: AsyncRead + Unpin>(reader: &mut BufReader<R>) -> Result<Option<Value>> {
    let mut bytes = Vec::new();
    loop {
        let buffer = reader.fill_buf().await?;
        if buffer.is_empty() {
            return if bytes.is_empty() {
                Ok(None)
            } else {
                Err(anyhow!("incomplete frame"))
            };
        }
        let end = buffer.iter().position(|b| *b == b'\n');
        let count = end.map_or(buffer.len(), |i| i + 1);
        if bytes.len() + count > 1_048_576 {
            return Err(anyhow!("frame exceeds 1 MiB"));
        }
        bytes.extend_from_slice(&buffer[..count]);
        reader.consume(count);
        if end.is_some() {
            return Ok(Some(serde_json::from_slice(&bytes)?));
        }
    }
}
pub async fn write_frame<W: AsyncWrite + Unpin>(writer: &mut W, value: &Value) -> Result<()> {
    let mut bytes = serde_json::to_vec(value)?;
    bytes.push(b'\n');
    tokio::time::timeout(std::time::Duration::from_secs(10), writer.write_all(&bytes)).await??;
    Ok(())
}
async fn connection(pipe: NamedPipeServer, runtime: Arc<Mutex<Runtime>>) -> Result<()> {
    let (reader, mut writer) = tokio::io::split(pipe);
    let mut reader = BufReader::new(reader);
    while let Some(request) = read_frame(&mut reader).await? {
        let request_id = request["id"].clone();
        let operation = request["method"].as_str().unwrap_or_default();
        let value = request["params"].clone();
        if operation == "terminal.attach" {
            let pane_id = value["paneId"].as_str().unwrap_or_default();
            let output = runtime
                .lock()
                .unwrap()
                .terminals
                .get(pane_id)
                .map(|terminal| terminal.output.clone());
            let Some(output) = output else {
                write_frame(
                    &mut writer,
                    &json!({"id":request_id,"ok":false,"error":"terminal unavailable"}),
                )
                .await?;
                continue;
            };
            let (mut receiver, snapshot, exited) = {
                let output = output.lock().unwrap();
                (
                    output.sender.subscribe(),
                    output.parser.screen().state_formatted(),
                    output.exited,
                )
            };
            write_frame(&mut writer, &json!({"id":request_id,"ok":true,"result":{"data":STANDARD.encode(snapshot),"exited":exited}})).await?;
            if exited {
                return Ok(());
            }
            loop {
                tokio::select! {
                    _ = read_frame(&mut reader) => return Ok(()),
                    event = receiver.recv() => {
                        let value = match event {
                            Ok(data) if data.is_empty() => json!({"event":"terminal.exited","paneId":pane_id}),
                            Ok(data) => json!({"event":"pane.output","paneId":pane_id,"data":STANDARD.encode(data)}),
                            Err(tokio::sync::broadcast::error::RecvError::Lagged(_)) => return Err(anyhow!("slow terminal client; reconnect to recover screen")),
                            Err(_) => return Ok(()),
                        };
                        write_frame(&mut writer, &value).await?;
                        if value["event"] == "terminal.exited" { return Ok(()); }
                    }
                }
            }
        }
        let response = match runtime.lock().unwrap().dispatch(operation, value) {
            Ok(result) => json!({"id":request_id,"ok":true,"result":result}),
            Err(error) => json!({"id":request_id,"ok":false,"error":format!("{error:#}")}),
        };
        write_frame(&mut writer, &response).await?;
    }
    Ok(())
}
pub async fn serve(name: &str, path: &std::path::Path) -> Result<()> {
    let mut pipe = server(name, true)?;
    let runtime = Arc::new(Mutex::new(Runtime::open(path, name.into())?));
    loop {
        pipe.connect().await?;
        let connected = pipe;
        pipe = server(name, false)?;
        let runtime = runtime.clone();
        tokio::spawn(async move {
            if let Err(error) = connection(connected, runtime).await {
                eprintln!("client disconnected: {error}");
            }
        });
    }
}
pub async fn call(name: &str, method: &str, params: Value) -> Result<Value> {
    let mut pipe = ClientOptions::new().open(format!(r"\\.\pipe\{name}"))?;
    write_frame(&mut pipe, &json!({"id":1,"method":method,"params":params})).await?;
    let response = read_frame(&mut BufReader::new(pipe))
        .await?
        .ok_or_else(|| anyhow!("runtime disconnected"))?;
    if response["ok"] != true {
        return Err(anyhow!(
            "{}",
            response["error"].as_str().unwrap_or("request failed")
        ));
    }
    Ok(response["result"].clone())
}

#[cfg(test)]
mod tests {
    use super::*;
    #[tokio::test]
    async fn framing_handles_multiple_messages_and_unicode() {
        let data = "{\"data\":\"你好\"}\n{\"id\":2}\n";
        let mut reader = BufReader::new(data.as_bytes());
        assert_eq!(
            read_frame(&mut reader).await.unwrap().unwrap()["data"],
            "你好"
        );
        assert_eq!(read_frame(&mut reader).await.unwrap().unwrap()["id"], 2);
        assert!(read_frame(&mut reader).await.unwrap().is_none());
    }
    #[tokio::test]
    async fn framing_rejects_unbounded_and_truncated_input() {
        let data = vec![b'x'; 1_048_577];
        assert!(read_frame(&mut BufReader::new(data.as_slice()))
            .await
            .is_err());
        assert!(read_frame(&mut BufReader::new(&b"{\"id\":1}"[..]))
            .await
            .is_err());
    }
}
