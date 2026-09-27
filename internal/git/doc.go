// Package git runs git as a subprocess and parses its porcelain output into
// values. It never reimplements git; see the "Replace git" non-goal in
// docs/design/product.md.
//
// Decision/effect boundary: the porcelain parsers are pure functions over
// bytes, tested without a repository. Only a thin runner at the edge executes
// the process.
package git
