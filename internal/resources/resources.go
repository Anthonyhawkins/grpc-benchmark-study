package resources

import "embed"

//go:embed certs/*
var Certs embed.FS

//go:embed cms/*
var CMS embed.FS

//go:embed jwt/*
var JWT embed.FS
