package shell

import "os"

// WriteDirective replaces the contents of the directive file at path with
// dest, the destination's native absolute path, and nothing else: no
// trailing newline.
//
// The function creates the file; the binary never does. A path that does not
// exist is an error, and no file is created there.
func WriteDirective(path, dest string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(dest); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
