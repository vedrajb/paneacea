#[derive(Default)]
pub struct Queries {
    pending: Vec<u8>,
    string: bool,
    escape_in_string: bool,
    osc_title: bool,
    string_buffer: Vec<u8>,
}
impl Queries {
    pub fn process(&mut self, data: &[u8], parser: &mut vt100::Parser) -> (Vec<u8>, Vec<u8>) {
        let (output, replies, _) = self.process_with_title(data, parser);
        (output, replies)
    }
    pub fn process_with_title(
        &mut self,
        data: &[u8],
        parser: &mut vt100::Parser,
    ) -> (Vec<u8>, Vec<u8>, Option<String>) {
        let mut output = Vec::new();
        let mut replies = Vec::new();
        let mut title = None;
        let mut parsed = 0;
        for &byte in data {
            if self.string {
                let terminated = byte == 7 || (self.escape_in_string && byte == b'\\');
                if self.osc_title {
                    if terminated {
                        if self.escape_in_string
                            && byte == b'\\'
                            && self.string_buffer.last() == Some(&27)
                        {
                            self.string_buffer.pop();
                        }
                        title = osc_title(&self.string_buffer);
                    } else if self.string_buffer.len() < 4096 {
                        self.string_buffer.push(byte);
                    }
                }
                output.push(byte);
                if terminated {
                    self.string = false;
                    self.osc_title = false;
                    self.string_buffer.clear();
                }
                self.escape_in_string = byte == 27;
                continue;
            }
            if self.pending.is_empty() {
                if byte == 27 {
                    self.pending.push(byte);
                } else {
                    output.push(byte);
                }
                continue;
            }
            self.pending.push(byte);
            if self.pending.len() == 2 {
                if byte == b'[' {
                    continue;
                }
                if byte == b']' {
                    self.string = true;
                    self.osc_title = true;
                    self.escape_in_string = false;
                    self.string_buffer.clear();
                } else if [b'P', b'X', b'^', b'_'].contains(&byte) {
                    self.string = true;
                    self.osc_title = false;
                    self.escape_in_string = false;
                    self.string_buffer.clear();
                }
                output.append(&mut self.pending);
                continue;
            }
            if (0x40..=0x7e).contains(&byte) || self.pending.len() >= 128 {
                parser.process(&output[parsed..]);
                parsed = output.len();
                match self.pending.as_slice() {
                    b"\x1b[6n" | b"\x1b[?6n" => {
                        let (row, column) = parser.screen().cursor_position();
                        let prefix = if self.pending[2] == b'?' { "?" } else { "" };
                        replies.extend_from_slice(
                            format!("\x1b[{prefix}{};{}R", row + 1, column + 1).as_bytes(),
                        );
                        self.pending.clear();
                    }
                    b"\x1b[5n" => {
                        replies.extend_from_slice(b"\x1b[0n");
                        self.pending.clear();
                    }
                    b"\x1b[c" | b"\x1b[0c" => {
                        replies.extend_from_slice(b"\x1b[?1;2c");
                        self.pending.clear();
                    }
                    b"\x1b[>c" | b"\x1b[>0c" => {
                        replies.extend_from_slice(b"\x1b[>0;1;0c");
                        self.pending.clear();
                    }
                    _ => output.append(&mut self.pending),
                }
            }
        }
        parser.process(&output[parsed..]);
        (output, replies, title)
    }
}
fn osc_title(data: &[u8]) -> Option<String> {
    let separator = data.iter().position(|byte| *byte == b';')?;
    if !matches!(data.get(..separator), Some(b"0" | b"1" | b"2")) {
        return None;
    }
    let title = String::from_utf8_lossy(&data[separator + 1..])
        .trim()
        .to_string();
    (!title.is_empty()).then_some(title)
}
#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn fragmented_queries_use_runtime_cursor_and_are_not_forwarded() {
        let mut queries = Queries::default();
        let mut parser = vt100::Parser::new(30, 120, 0);
        assert_eq!(
            queries.process(b"abc\x1b[", &mut parser),
            (b"abc".to_vec(), vec![])
        );
        assert_eq!(
            queries.process(b"6n", &mut parser),
            (vec![], b"\x1b[1;4R".to_vec())
        );
        assert_eq!(
            queries.process(b"\x1b[5n\x1b[c", &mut parser).1,
            b"\x1b[0n\x1b[?1;2c"
        );
    }
    #[test]
    fn text_colors_and_osc_are_preserved() {
        let mut queries = Queries::default();
        let mut parser = vt100::Parser::new(30, 120, 0);
        let data = "\x1b]0;title\x07\x1b[32m你好\x1b[0m".as_bytes();
        let (output, replies) = queries.process(data, &mut parser);
        assert_eq!(output, data);
        assert!(replies.is_empty());
        assert_eq!(parser.screen().contents(), "你好");
    }
}
