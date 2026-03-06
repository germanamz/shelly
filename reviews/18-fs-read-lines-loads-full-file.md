# `fs_read_lines` Loads Entire File Before Slicing

## Severity: Minor

## Location

- `pkg/codingtoolbox/filesystem/filesystem.go:268-309`

## Description

`handleReadLines` reads the entire file into memory (up to 10MB) using `io.ReadAll` before splitting into lines and returning the requested range.

The tool is described in the README as "useful for inspecting large files without loading the full content." For files approaching the 10MB cap, the memory behavior is identical to `fs_read` regardless of how narrow the requested line range is.

## Fix

Use a streaming `bufio.Scanner` to read only as many lines as needed to reach the end of the requested range, avoiding full file allocation.
