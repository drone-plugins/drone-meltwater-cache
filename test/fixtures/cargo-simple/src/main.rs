fn main() {
    let mut buffer = itoa::Buffer::new();
    assert_eq!(buffer.format(42), "42");
}
