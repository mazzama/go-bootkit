module github.com/mazzama/go-bootkit/serverkit

go 1.24.6

replace github.com/mazzama/go-bootkit/core => ../core

require (
	github.com/go-chi/chi/v5 v5.2.3
	github.com/go-chi/httplog/v3 v3.2.2
	github.com/mazzama/go-bootkit/core v0.0.0-00010101000000-000000000000
)
