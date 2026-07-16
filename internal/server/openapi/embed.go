// Package openapi holds cueBreaker's hand-written API specification and the
// vendored Scalar bundle that renders it, both embedded into the binary.
package openapi

import _ "embed"

// Spec is the OpenAPI document describing every /api/ route. It is written by
// hand; internal/server's drift test keeps it in step with the routes the
// server actually registers.
//
//go:embed openapi.yaml
var Spec []byte

// ScalarJS is the Scalar standalone bundle that renders Spec. It is vendored
// rather than loaded from a CDN so the reference page works on a network that
// can only reach this stack.
//
//go:embed scalar.js
var ScalarJS []byte

// SpecURL is where the server serves Spec, and where DocsHTML points Scalar.
const SpecURL = "/api/openapi.yaml"

// BundleURL is where the server serves ScalarJS.
const BundleURL = "/api/docs/scalar.js"

// DocsHTML is the reference page: Scalar reads the spec URL off the script
// tag's data-url attribute, then the bundle mounts itself over the tag.
const DocsHTML = `<!doctype html>
<html>
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>cueBreaker API</title>
  </head>
  <body>
    <script id="api-reference" data-url="` + SpecURL + `"></script>
    <script src="` + BundleURL + `"></script>
  </body>
</html>
`
