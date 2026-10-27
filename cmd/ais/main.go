package main

import (
	"github.com/harishtpj/ais/internal/cli"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()
	cli.Execute()
}
