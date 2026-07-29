package main

import (
	"context"

	"github.com/Omotolani98/framesctl/internals/commands"
)

func main() {
	commands.Execute(context.Background())
}
