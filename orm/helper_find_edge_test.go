package orm

import "testing"

func TestJoinTargetsTableMatchesOnlyTheRequestedJoinTarget(t *testing.T) {
	tests := []struct {
		name       string
		join       string
		table      string
		wantTarget bool
	}{
		{
			name:       "plain table",
			join:       "LEFT JOIN authors ON authors.id = posts.author_id",
			table:      "authors",
			wantTarget: true,
		},
		{
			name:       "qualified table",
			join:       "INNER JOIN app.authors ON authors.id = posts.author_id",
			table:      "authors",
			wantTarget: true,
		},
		{
			name:       "case insensitive table",
			join:       "JOIN AUTHORS ON AUTHORS.id = posts.author_id",
			table:      "authors",
			wantTarget: true,
		},
		{
			name:       "different target",
			join:       "JOIN authors_archive ON authors_archive.id = posts.author_id",
			table:      "authors",
			wantTarget: false,
		},
		{
			name:       "table text outside join target",
			join:       "JOIN posts ON posts.author_id = authors.id",
			table:      "authors",
			wantTarget: false,
		},
		{
			name:       "regex metacharacters in table name",
			join:       "JOIN audit.logs ON true",
			table:      "audit.logs",
			wantTarget: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := joinTargetsTable(test.join, test.table); got != test.wantTarget {
				t.Fatalf("joinTargetsTable(%q, %q) = %t, want %t",
					test.join, test.table, got, test.wantTarget)
			}
		})
	}
}

func TestFindSelectedColsNormalizeStructInputMongo(t *testing.T) {
	tests := []struct {
		name     string
		selected []string
		column   string
		field    string
		want     string
		wantOK   bool
	}{
		{
			name:     "matches database column case insensitively",
			selected: []string{"EMAIL"},
			column:   "email",
			field:    "EmailAddress",
			want:     "email",
			wantOK:   true,
		},
		{
			name:     "matches struct field name",
			selected: []string{"emailaddress"},
			column:   "email",
			field:    "EmailAddress",
			want:     "email",
			wantOK:   true,
		},
		{
			name:     "rejects unselected field",
			selected: []string{"name"},
			column:   "email",
			field:    "EmailAddress",
			want:     "email",
			wantOK:   false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := findSelectedColsNormalizeStructInputMongo(test.selected, test.column, test.field)
			if got != test.want || ok != test.wantOK {
				t.Fatalf("findSelectedColsNormalizeStructInputMongo() = (%q, %t), want (%q, %t)",
					got, ok, test.want, test.wantOK)
			}
		})
	}
}
