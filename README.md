# Description
This lambda is intended to listen for S3 events and then generate index html
files for all the objects in the bucket.


Can also be run from the command line. If running from a command line the
binary takes one argument which is the bucket name.

# Environment Variables

| Name                    | Required | Default                         | Description |
|-------------------------|----------|---------------------------------|-------------|
| `BUCKET`                | Yes (Lambda) | —                           | The S3 bucket to index. In CLI mode this is the first argument. |
| `DESTINATION_BUCKET_PREFIX` | No   |                                 | Output prefix within the bucket for generated indexes. |
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
If `TEMPLATE_BUCKET_URL` is set the utility will look for a root template with the name `${INDEX_TYPE}.index.html.tmpl` within a subdirectory of `TEMPLATE_BUCKET_URL`

Templates are rendered against an `ObjectTree` instance:

```
type Object struct {
    Object *s3.Object
    Tags map[string]string
}

type ObjectTree struct {
	FullPath string
	DirName  string
	Objects  []*Object
	Children map[string]*ObjectTree
}
```

Each `ObjectTree` represents a directory within the `SRC_BUCKET_NAME` bucket.
Objects 'in' the directory are contained in the `Objects` member, while sub
trees are contained within `Children`. `DirName` is the base name of a path,
i.e. if the path is `a/b/c` the basename is `c`. The `FullPath` contains the
full path to the folder, in the previous example it would be `a/b/c`.

