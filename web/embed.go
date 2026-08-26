package web

import "embed"

// Files contains ApoFocus' dependency-free web application.
//
//go:embed static/*
var Files embed.FS
