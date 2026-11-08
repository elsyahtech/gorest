package hardening

import (
	"bytes"
	"html/template"
)

var reportHTML = template.Must(template.New("hardening-report").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>GOREST hardening report</title>
<style>
body{font:16px system-ui,sans-serif;line-height:1.5;margin:2rem auto;max-width:1100px;padding:0 1rem;color:#17202a}
table{border-collapse:collapse;width:100%;margin:1rem 0 2rem}th,td{border:1px solid #ccd1d1;padding:.55rem;text-align:left;vertical-align:top}
th{background:#f2f4f4}code{overflow-wrap:anywhere}.fail{color:#a93226}.review{color:#9a7d0a}.pass{color:#196f3d}.unknown,.not_applicable{color:#626567}
</style>
</head>
<body>
<h1>GOREST hardening report</h1>
<p><strong>Profile:</strong> {{.Profile}} · <strong>Evidence:</strong> {{.EvidenceMode}} · <strong>Generated:</strong> {{.GeneratedAt}}</p>
<p>{{.Warning}}</p>
<h2>Summary</h2>
<p>Fail: {{.Summary.Fail}} · Review: {{.Summary.Review}} · Pass: {{.Summary.Pass}} · Unknown: {{.Summary.Unknown}} · Not applicable: {{.Summary.NotApplicable}}</p>
<h2>Findings</h2>
<table><thead><tr><th>ID</th><th>Status</th><th>Severity</th><th>Field</th><th>Observed</th><th>Finding and recommendation</th></tr></thead><tbody>
{{range .Findings}}<tr><td>{{.ID}}</td><td class="{{.Status}}">{{.Status}}</td><td>{{.Severity}}</td><td><code>{{.FieldPath}}</code></td><td><code>{{printf "%v" .ObservedValue}}</code></td><td>{{.Message}} {{.Recommendation}}</td></tr>{{else}}<tr><td colspan="6">No applicable checks produced findings. This is not a security certification.</td></tr>{{end}}
</tbody></table>
<h2>Configuration inventory</h2>
<table><thead><tr><th>Field</th><th>Value</th><th>Assessment</th></tr></thead><tbody>
{{range .Inventory}}<tr><td><code>{{.Path}}</code></td><td><code>{{printf "%v" .Value}}</code></td><td class="{{.Status}}">{{.Status}}</td></tr>{{end}}
</tbody></table>
</body>
</html>`))

// RenderHTML encodes a report as escaped HTML.
func RenderHTML(report Report) ([]byte, error) {
	var output bytes.Buffer
	if err := reportHTML.Execute(&output, report); err != nil {
		return nil, err
	}

	return output.Bytes(), nil
}
