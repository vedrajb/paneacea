use crate::model::Pane;
use anyhow::{Context, Result};
use portable_pty::{native_pty_system, Child, CommandBuilder, MasterPty, PtySize};
use std::{
    io::{Read, Write},
    sync::{Arc, Mutex},
};
use tokio::sync::broadcast;

pub struct Output {
    pub parser: vt100::Parser,
    pub sender: broadcast::Sender<Vec<u8>>,
    pub exited: bool,
}
pub struct Terminal {
    pub master: Box<dyn MasterPty + Send>,
    pub writer: Arc<Mutex<Box<dyn Write + Send>>>,
    pub child: Box<dyn Child + Send + Sync>,
    pub output: Arc<Mutex<Output>>,
}
impl Terminal {
    pub fn launch(pane: &mut Pane, pipe: &str) -> Result<Self> {
        let pair = native_pty_system().openpty(PtySize {
            rows: 30,
            cols: 120,
            pixel_width: 0,
            pixel_height: 0,
        })?;
        let mut command = CommandBuilder::new(&pane.executable);
        command.args(&pane.arguments);
        command.cwd(&pane.current_working_directory);
        for (key, value) in &pane.environment {
            command.env(key, value);
        }
        command.env("TINKERSHELL_PIPE", pipe);
        command.env("TINKERSHELL_PANE_ID", &pane.id);
        command.env("TINKERSHELL_WORKSPACE_ID", &pane.workspace_id);
        let child = pair
            .slave
            .spawn_command(command)
            .context("could not launch terminal")?;
        pane.pid = child.process_id();
        pane.generation = crate::model::id();
        pane.error = None;
        drop(pair.slave);
        let mut reader = pair.master.try_clone_reader()?;
        let writer = Arc::new(Mutex::new(pair.master.take_writer()?));
        let (sender, _) = broadcast::channel(256);
        let output = Arc::new(Mutex::new(Output {
            parser: vt100::Parser::new(30, 120, 2000),
            sender,
            exited: false,
        }));
        let thread_output = output.clone();
        let thread_writer = writer.clone();
        std::thread::spawn(move || {
            let mut buffer = [0u8; 8192];
            let mut queries = crate::queries::Queries::default();
            loop {
                match reader.read(&mut buffer) {
                    Ok(0) | Err(_) => break,
                    Ok(count) => {
                        let mut output = thread_output.lock().unwrap();
                        let (data, replies) = queries.process(&buffer[..count], &mut output.parser);
                        if !data.is_empty() {
                            let _ = output.sender.send(data);
                        }
                        drop(output);
                        if !replies.is_empty() {
                            let mut writer = thread_writer.lock().unwrap();
                            if writer
                                .write_all(&replies)
                                .and_then(|_| writer.flush())
                                .is_err()
                            {
                                break;
                            }
                        }
                    }
                }
            }
            let mut output = thread_output.lock().unwrap();
            output.exited = true;
            let _ = output.sender.send(Vec::new());
        });
        Ok(Self {
            master: pair.master,
            writer,
            child,
            output,
        })
    }
    pub fn resize(&mut self, rows: u16, columns: u16) -> Result<()> {
        anyhow::ensure!(
            (1..=500).contains(&rows) && (1..=1000).contains(&columns),
            "invalid terminal dimensions"
        );
        self.master.resize(PtySize {
            rows,
            cols: columns,
            pixel_width: 0,
            pixel_height: 0,
        })?;
        self.output.lock().unwrap().parser.set_size(rows, columns);
        Ok(())
    }
    pub fn write(&mut self, data: &[u8]) -> Result<()> {
        let mut writer = self.writer.lock().unwrap();
        writer.write_all(data)?;
        writer.flush()?;
        Ok(())
    }
    pub fn close(&mut self) {
        let _ = self.child.kill();
        let _ = self.child.wait();
    }
}
impl Drop for Terminal {
    fn drop(&mut self) {
        self.close();
    }
}
