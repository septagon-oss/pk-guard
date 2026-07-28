// Command pk-guard is the standard pk-guard vettool: the Std analyzer set
// with no extensions.
//
// Repositories with their own analyzers write their own main instead — see
// package guardmain for the five-line composition pattern.
//
// Usage:
//
//	go run github.com/septagon-oss/pk-guard/cmd/pk-guard@latest ./...
//	go vet -vettool=$(which pk-guard) ./...
package main

import "github.com/septagon-oss/pk-guard/guardmain"

func main() {
	guardmain.Run(guardmain.Std()...)
}
