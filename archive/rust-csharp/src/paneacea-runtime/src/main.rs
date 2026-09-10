#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

fn default_data_directory() -> anyhow::Result<std::path::PathBuf> {
    let executable = std::env::current_exe()?;
    executable
        .parent()
        .map(|path| path.to_path_buf())
        .ok_or_else(|| anyhow::anyhow!("runtime executable has no parent directory"))
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    let mut name = paneacea_runtime::ipc::default_pipe();
    let mut directory = std::env::var_os("PANEACEA_DATA")
        .filter(|value| !value.is_empty())
        .map(std::path::PathBuf::from)
        .unwrap_or(default_data_directory()?);
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

#[cfg(test)]
mod tests {
    #[test]
    fn default_data_directory_uses_executable_directory() {
        let executable = std::env::current_exe().unwrap();
        assert_eq!(
            super::default_data_directory().unwrap(),
            executable.parent().unwrap()
        );
    }
}
