# Development

## Library checks

```sh
make test
```

This runs formatting, `go vet`, and race-enabled tests with coverage. CI checks
Go 1.24 and the current stable Go release. For an HTML coverage report:

```sh
make cover
```

## Examples

Run the examples from the repository root:

```sh
go run ./examples/single
go run ./examples/multiple
```

They use local HTTP test servers. Example webhook JSON and email HTML templates
are also available in the repository's
[examples directory](https://github.com/containeroo/notifykit/tree/main/examples).
The email template expects application-specific check-in data; adapt it to your
own notification type.

## Documentation source

The documentation uses [Lore](https://github.com/gi8lino/lore). Markdown pages live
in `docs/content`, with the home page at `docs/content/index.md`. Site settings
live in `docs/site.toml`; Lore's native configuration format is TOML.

Link to other source pages with relative Markdown links such as
`[Retries](retries.md)`. Lore rewrites these to generated page routes and fails the
build when a Markdown target is missing. The site includes search and Mermaid
support. Keep generated output out of version control.

## Build Lore

Use a Lore binary that includes its browser assets. Building only the Go command
from an unprepared source checkout is not enough: Lore also generates its icon
catalog and builds frontend assets. The following source revision
was used to validate this documentation:

```sh
git clone https://github.com/gi8lino/lore.git /tmp/lore
cd /tmp/lore
git checkout a10743ef927221ed491e316869b3ef1466f1945a
make build BINARY=/tmp/lore-bin
```

That revision requires Go 1.27 or newer and Node.js/npm to build its frontend.
These are documentation-tool requirements; the Notifykit library still supports
Go 1.24.

## Build the site

From the Notifykit repository root:

```sh
make docs LORE=/tmp/lore-bin
```

If `lore` is on your PATH, use `make docs`. The equivalent command is:

```sh
lore build --config docs/site.toml
```

Generated files go into `docs/site`. Source and output paths in Lore's config
are relative to the working directory, so run the command from the repository
root.

## Preview and publish

Preview the generated site locally:

```sh
python3 -m http.server 8000 --directory docs/site --bind 127.0.0.1
```

Open `http://127.0.0.1:8000`. The checked-in site URL is `/`, so the default build
works at a web server's root. Before publishing under a repository prefix or a
custom domain, pass the actual deployment URL:

```sh
make docs LORE=/tmp/lore-bin DOCS_SITE_URL=https://containeroo.github.io/notifykit/
```

This is an example deployment URL, not an assertion that the site is already
published. Upload `docs/site` to your static host after building. Lore emits a
`.nojekyll` file, and an absolute site URL also enables sitemap generation.

For configuration details, see
[Lore's static site guide](https://github.com/gi8lino/lore/blob/main/docs/content/static-sites.md).
