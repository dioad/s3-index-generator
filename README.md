# Description
This lambda listens for S3 events and generates index files (HTML and JSON) for the
objects in a bucket. Every run lists the whole bucket and rewrites all indexes, so one run
covers any number of uploaded objects.

Can also be run from the command line: `s3-index-generator <bucket>[/<destination-prefix>] [<local-output-directory>]`.
With a local output directory the indexes are written there instead of back to the bucket.
CLI runs write `cpu.pprof` and `heap.pprof` to the current directory.

# Triggering

The Lambda accepts either:

- an **S3 event notification**. Every record is examined and one rebuild runs if any of them
  qualifies. Events for keys outside `OBJECT_PREFIX`, for generated files (`index.html`,
  `index.json`) and for the generated `static/` assets are ignored, so the generator's own output
  cannot start another run.
- a **reindex request**, `{"reindex": true}`, which rebuilds everything without an S3 record and
  clears the template and static-asset caches. Use it after publishing a new template, which does
  not trigger a run on its own:

  ```
  aws lambda invoke --function-name <name> --invocation-type Event \
    --cli-binary-format raw-in-base64-out --payload '{"reindex":true}' /dev/null
  ```

`DESTINATION_BUCKET_PREFIX` must not be inside `OBJECT_PREFIX`; the generator refuses to start
otherwise. The deployed layouts are `OBJECT_PREFIX=data` with outputs at the bucket root, and
`OBJECT_PREFIX=private-data` with `DESTINATION_BUCKET_PREFIX=private`.

Overlapping runs rebuild from a full listing and last writer wins, so a stale run could overwrite a
newer index. Set the function's reserved concurrency to 1 to serialise them.

# Environment Variables

| Name                    | Required | Default                         | Description |
|-------------------------|----------|---------------------------------|-------------|
| `BUCKET`                | Yes (Lambda) | —                           | The S3 bucket to index. In CLI mode this is the first argument. |
| `DESTINATION_BUCKET_PREFIX` | No   |                                 | Output prefix within the bucket for generated indexes. Must not be inside `OBJECT_PREFIX`. |
| `OBJECT_PREFIX`         | No       |                                 | Only index objects whose key starts with this prefix. |
| `INDEX_TYPE`            | No       | `multipage`                     | `multipage` generates one index per directory; `singlepage` generates a single root index. |
| `INDEX_FORMATS`         | No       | `html,json`                     | Comma-separated list of output formats to generate. Supported: `html`, `json`. |
| `TEMPLATE_BUCKET_URL`   | No       |                                 | S3 URL in the form `s3://bucket/path`. Expects templates in a subdirectory called `templates/`. So if this is set to `s3://bucket/path` it will expect templates at `s3://bucket/path/templates/singlepage.index.html`. |
| `STATIC_BUCKET_URL`     | No       |                                 | S3 URL in the form `s3://bucket/path`. Expects static assets in a subdirectory called `static/`. So if this is set to `s3://bucket/path` it will expect assets at `s3://bucket/path/static/style.css`. |
| `INDEX_TEMPLATE`        | No       | `${INDEX_TYPE}.index.html.tmpl` | Override the template filename used for HTML rendering. |
| `SSE`                   | No       |                                 | Server-side encryption algorithm for output objects (e.g. `AES256`). |
| `RELEASE_KEY_PATTERNS`  | No       | built-in default                | Comma-separated list of named-capture-group regex patterns used to extract release metadata from S3 object keys. The first pattern to match an object key wins. When unset, the built-in default pattern is used (see below). |

# Custom Release Key Patterns

By default, the generator extracts release metadata (product, version, OS, arch) from S3 keys
matching this layout:

```
[prefix/]<product>/<version>/<package>[_<extra>_]<os>_<arch>.<ext>
```

Examples:
- `data/connect/0.57.1/connect_linux_amd64.tar.gz`
- `data/connect/0.57.1/dioad-connect_0.57.1_linux_amd64.deb`

To override this, set `RELEASE_KEY_PATTERNS` to a comma-separated list of Go named-capture-group
regexes. Each pattern **must** include named groups `Product`, `Version`, `OS`, and `Arch`.
The first pattern that matches a key wins. Example:

```
RELEASE_KEY_PATTERNS=(?P<Product>[^/]+)/(?P<Version>[^/]+)/(?P<OS>[^_]+)_(?P<Arch>[^.]+)\.zip
```

# Custom Templates
If `TEMPLATE_BUCKET_URL` is set the utility loads templates from the `templates/` subdirectory of
that URL instead of the embedded ones, and `STATIC_BUCKET_URL` likewise supplies the `static/`
assets, which are copied to `static/` under the destination. The root template is named
`${INDEX_TYPE}.index.html.tmpl` unless `INDEX_TEMPLATE` is set. Every file under `templates/` is
parsed, so a root template can `{{template "other.tmpl" .}}` the others.

Both URLs can point at the bucket being indexed. Keep them under a key starting with `.`
(for example `s3://my-bucket/.templates`): keys beginning with `.` are never indexed, and, being
outside `OBJECT_PREFIX`, uploading them does not trigger a run. Send a reindex request afterwards.

The lambda role needs `s3:GetObject` and `s3:ListBucket` on the template and static locations.
Missing files are only reported as such when `s3:ListBucket` is granted; without it S3 answers
`AccessDenied` and the run fails. Templates and static assets are cached while a warm Lambda sees
the same ETags, so changes take effect on the next run.

Templates are executed with a `Page`:

```go
type Page struct {
	Nonce      string      // random value for a Content-Security-Policy nonce
	ObjectTree *ObjectTree
}

type ObjectTree struct {
	FullPath string                 // e.g. "/a/b/c"; the root is "/"
	DirName  string                 // base name, e.g. "c"
	Objects  []Object               // objects directly in this directory
	Children map[string]*ObjectTree // sub-directories
}

// SortedChildren returns Children in display order: folders that are not versions first, by name,
// then version folders by semantic version, newest first (1.10.0 before 1.9.0).
func (t *ObjectTree) SortedChildren() []*ObjectTree

type Object interface {
	Key() string               // full S3 key
	BaseName() string
	LastModified() time.Time
	Size() int64               // bytes
	Tags() map[string]string   // not populated by the generator
}
```

Each `ObjectTree` is one directory of the bucket (below `OBJECT_PREFIX`). `DirName` is the base name
of the path and `FullPath` the whole path.
Range over `.SortedChildren` rather than `.Children` to list sub-directories: ranging over the map
gives plain string order, which puts `1.10.0` before `1.9.0`.

In addition to the standard `html/template` functions, `humanize` formats a byte count with binary
units:

```
<a href="/{{ .Key }}">{{ .BaseName }}</a> {{ .LastModified.Format "2006-01-02 15:04" }} {{ humanize .Size }}
```

`humanize 1536` renders `1.5 KiB`.

# JSON Index

`index.json` is written only for directories that hold releases (an archive, product or version
directory); other directories get no `index.json`. A product index contains `versions`, `latest`
(the highest version) and `releases` (the highest version of each major version, keyed by major
number). Each build lists `arch`, `os`, `filename`, `url`, `version` and `size` in bytes. Objects
that match none of the `RELEASE_KEY_PATTERNS` are left out and counted in the log.
