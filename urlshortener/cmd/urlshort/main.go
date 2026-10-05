// Command urlshort is the entrypoint for the URL shortener. It only dispatches
// to the Cobra CLI; all logic lives in internal packages.
package main

import "github.com/yazdanjavadi/urlshort/internal/cli"

func main() {
	cli.Execute()
}
