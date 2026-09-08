#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    let mut name = paneacea_runtime::ipc::default_pipe();
    let mut directory = std::path::PathBuf::from(std::env::var("LOCALAPPDATA")?).join("Paneacea");
    let mut arguments = std::env::args().skip(1);
    while let Some(argument) = arguments.next() {
        match argument.as_str() {
            "--pipe" => {
                name = arguments
                    .next()
                    .ok_or_else(|| anyhow::anyhow!("--pipe requires a name"))?
            }
            "--data" => {
                directory = arguments
                    .next()
                    .ok_or_else(|| anyhow::anyhow!("--data requires a directory"))?
                    .into()
            }
            _ => anyhow::bail!("usage: panacea-runtime [--pipe NAME] [--data DIRECTORY]"),
        }
    }
    std::fs::create_dir_all(&directory)?;
    paneacea_runtime::ipc::serve(&name, &directory.join("paneacea.db")).await
}
