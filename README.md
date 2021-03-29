
# Environment Variables

| Name                  | Required | Default                         | Description |
|-----------------------|----------|---------------------------------|-------------|
| `SRC_BUCKET_NAME`     | Yes      |                                 |             |
| `DEST_BUCKET_NAME`    | No       |                                 |             |
| `INDEX_TYPE`          | No       | `multipage`                     |             |
| `TEMPLATE_BUCKET_URL` | No       |                                 | S3 URL in the form `s3://bucket/path`. Expects templates (defined below) in a subdirectory called `templates/`. So if this is set to `s3://bucket/path` it will expect templates to be stored in `s3://bucket/path/templates/singlepage.index.html` |
| `INDEX_TEMPLATE`      | No       | `${INDEX_TYPE}.index.html.tmpl` |             |

# Custom Templates
If `TEMPLATE_BUCKET_URL` is set the utility will look for a root template with the name `${INDEX_TYPE}.index.html.tmpl` within a subdirectory of `TEMPLATE_BUCKET_URL`

Templates are rendered against an `ObjectTree` instance:

```
type ObjectTree struct {
	FullPath string
	DirName  string
	Objects  []*s3.Object
	Children map[string]*ObjectTree
}
```

Each `ObjectTree` represents a directory within the `SRC_BUCKET_NAME` bucket.
Objects 'in' the directory are contained in the `Objects` member, while sub
trees are contained within `Children`. `DirName` is the base name of a path,
i.e. if the path is `a/b/c` the basename is `c`. The `FullPath` contains the
full path to the folder, in the previous example it would be `a/b/c`.

