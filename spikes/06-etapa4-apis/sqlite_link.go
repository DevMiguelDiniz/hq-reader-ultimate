package main

// Linka o driver no exe do Wails so para medir o tamanho final do binario
// (Wails + modernc.org/sqlite) com CGO_ENABLED=0.
import _ "modernc.org/sqlite"
