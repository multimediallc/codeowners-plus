# Tests

All unit tests should use Go standard library `testing` framework, not any testing dependencies.

Expected results should be named `expected` or use `expected` as a prefix.

If a function or data structure requires mocks and does not already use dependency injection,
consider adding dependency injection via interfaces to improve testability.

For http request mocking, use standard library `httptest` to create the server.

Use `tt` as the name for a table of tests. Use `tc` as the name for individual test cases from a `tt` test table.  Use `name` as the struct field representing the test name.  Example:

```go
tt := []struct{
  name: string,
  ...
} {
  ...
}
for _, tc := range tt {
  t.Run(tc.name, func(t *testing.T) ...)
}
```
