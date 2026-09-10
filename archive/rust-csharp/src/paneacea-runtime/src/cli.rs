#[tokio::main]
async fn main() -> anyhow::Result<()> {
    let mut arguments = std::env::args().skip(1);
    let mut name = paneacea_runtime::ipc::default_pipe();
    let mut method = arguments.next().unwrap_or_default();
    if method == "--pipe" {
        name = arguments
            .next()
            .ok_or_else(|| anyhow::anyhow!("missing pipe name"))?;
        method = arguments.next().unwrap_or_default();
    }
    if method.is_empty() || method == "--help" {
        println!("Paneacea protocol client\nUsage: paneacea [--pipe NAME] METHOD [JSON]\nExample: paneacea workspace.list\nSee doc/protocol.md for operations.");
        return Ok(());
    }
    let params = serde_json::from_str(&arguments.next().unwrap_or_else(|| "{}".into()))?;
    let result = paneacea_runtime::ipc::call(&name, &method, params).await?;
    println!("{}", serde_json::to_string_pretty(&result)?);
    Ok(())
}
