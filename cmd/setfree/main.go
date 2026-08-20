// Command setfree launches the coding CLI you already use, configured for
// the LLM gateway you choose.
package main

import (
	"os"

	"github.com/mindsdb/setfree/internal/app"
)

func main() {
	os.Exit(app.Run(os.Args[1:]))
}
