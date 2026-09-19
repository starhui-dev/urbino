package migrate

import "embed"

// embeddedMigrations is packaged with the binary so migrate and serve do not
// depend on the caller's working directory or a repository checkout.
//
//go:embed migrations/*.sql
var embeddedMigrations embed.FS

func LoadEmbedded() ([]Migration, error) {
	return loadFS(embeddedMigrations, "migrations")
}
