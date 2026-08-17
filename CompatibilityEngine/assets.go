package main

import (
	"embed"
)

//go:embed web/* assets/*
var embedded embed.FS
