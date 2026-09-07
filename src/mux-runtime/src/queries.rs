#[derive(Default)]
pub struct Queries {
    pending: Vec<u8>,
    string: bool,
    escape_in_string: bool,
}
impl Queries {
    pub fn process(&mut self, data: &[u8], parser: &mut vt100::Parser) -> (Vec<u8>, Vec<u8>) {
        let mut output = Vec::new();
        let mut replies = Vec::new();
        let mut parsed = 0;
        for &byte in data {
            if self.string {
                output.push(byte);
                if byte == 7 || (self.escape_in_string && byte == b'\\') {
                    self.string = false;
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
                if [b']', b'P', b'X', b'^', b'_'].contains(&byte) {
                    self.string = true;
                    self.escape_in_string = false;
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
        (output, replies)
    }
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
