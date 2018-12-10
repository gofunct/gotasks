package main

import "github.com/gofunct/service"

func main() {
	service.Execute(
		service.WithConfig("PWD", "example"),
		service.WithRootInfo("example", "this is a short descrption"),
		service.WithServeFunc(":8000"),
		service.WithDialFunc(":8000"),
	)
}
