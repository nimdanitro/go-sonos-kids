// Package ui embeds the built React app so the server ships as a single binary.
//
// `go generate ./ui` installs dependencies and builds the app into ./build.
// During development run `yarn dev` in this directory instead (Vite dev server
// with hot reload, proxying /api to a locally running `sonoskids serve`).
package ui

import "embed"

//go:generate yarn install
//go:generate yarn build

// Assets holds the contents of build/. The "all:" prefix keeps the committed
// .gitkeep embeddable, so `go build` works before the UI has been built.
//
//go:embed all:build
var Assets embed.FS

// Build is the directory inside Assets that holds the app.
const Build = "build"
