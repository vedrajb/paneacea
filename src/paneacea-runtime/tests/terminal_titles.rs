use paneacea_runtime::queries::Queries;

#[test]
fn extracts_osc_title_and_preserves_terminal_bytes() {
    let mut queries = Queries::default();
    let mut parser = vt100::Parser::new(30, 120, 0);
    let data = b"\x1b]0;PowerShell\x07";

    let (output, replies, title) = queries.process_with_title(data, &mut parser);

    assert_eq!(output, data);
    assert!(replies.is_empty());
    assert_eq!(title.as_deref(), Some("PowerShell"));
}

#[test]
fn extracts_title_when_st_terminator_is_fragmented() {
    let mut queries = Queries::default();
    let mut parser = vt100::Parser::new(30, 120, 0);

    let (_, _, title) = queries.process_with_title(b"\x1b]2;node", &mut parser);
    assert!(title.is_none());

    let (_, _, title) = queries.process_with_title(b"\x1b\\", &mut parser);
    assert_eq!(title.as_deref(), Some("node"));
}
