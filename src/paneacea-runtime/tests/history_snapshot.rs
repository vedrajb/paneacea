use paneacea_runtime::pty::{history_snapshot, Output};
use tokio::sync::broadcast;

fn output(rows: u16, columns: u16, contents: &[u8]) -> Output {
    let (sender, _) = broadcast::channel(4);
    let mut output = Output {
        parser: vt100::Parser::new(rows, columns, 100),
        sender,
        exited: false,
        title: String::new(),
        cwd: None,
        dirty: true,
        viewport_offset: 0,
        restored: false,
    };
    output.parser.process(contents);
    output
}

#[test]
fn snapshot_omits_trailing_screen_rows_and_cursor_position() {
    let mut output = output(30, 80, b"saved prompt");

    let history = history_snapshot(&mut output, 100);
    let snapshot = String::from_utf8_lossy(&history.data);

    assert!(snapshot.contains("saved prompt"));
    assert!(!snapshot.contains("\r\n\r\n"));
    assert!(!snapshot.contains(";12H"));
}

#[test]
fn restored_history_stays_at_the_top_when_terminal_height_changes() {
    let mut output = output(30, 80, b"saved prompt");
    let history = history_snapshot(&mut output, 100);
    let mut restored = vt100::Parser::new(history.rows, history.columns, 100);

    restored.process(&history.data);
    restored.process(b"\r\n--- restored ---\r\nnew prompt");
    restored.set_size(50, 80);

    let contents = restored.screen().contents();
    let lines = contents.lines().collect::<Vec<_>>();
    assert_eq!(lines.first().copied(), Some("saved prompt"));
    assert_eq!(lines.get(1).copied(), Some("--- restored ---"));
    assert_eq!(lines.get(2).copied(), Some("new prompt"));
}
