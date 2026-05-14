package app

import "testing"

func TestUsagePrinters(t *testing.T) {
	t.Parallel()

	printers := []func(){
		printUsage,
		printArticlesUsage,
		printDaemonUsage,
		printDeleteUsage,
		printPersonIdentitiesUsage,
		printRestoreUsage,
		printTagsUsage,
		printTranslateUsage,
		printUpdateUsage,
	}
	for _, printUsageFn := range printers {
		printUsageFn()
	}
}

func TestRunCommandsStopOnHelpOrValidationBeforeIO(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		run  func([]string) int
		args []string
		want int
	}{
		{name: "stories help", run: runStories, args: []string{"--help"}},
		{name: "stats help", run: runStats, args: []string{"--help"}},
		{name: "story detail help", run: runStoryDetail, args: []string{"--help"}},
		{name: "collections help", run: runCollections, args: []string{"--help"}},
		{name: "search missing query", run: runSearch, args: nil, want: 2},
		{name: "articles help", run: runArticles, args: []string{"help"}},
		{name: "articles add person usage", run: runArticlesAddPerson, args: nil, want: 2},
		{name: "articles remove person usage", run: runArticlesRemovePerson, args: nil, want: 2},
		{name: "articles list people usage", run: runArticlesListPeople, args: nil, want: 2},
		{name: "digest help", run: runDigest, args: []string{"--help"}},
		{name: "health help", run: runHealth, args: []string{"--help"}},
		{name: "ingest help", run: runIngest, args: []string{"--help"}},
		{name: "validate help", run: runValidate, args: []string{"--help"}},
		{name: "normalize help", run: runNormalize, args: []string{"--help"}},
		{name: "embed help", run: runEmbed, args: []string{"--help"}},
		{name: "dedup help", run: runDedup, args: []string{"--help"}},
		{name: "process help", run: runProcess, args: []string{"--help"}},
		{name: "serve help", run: runServe, args: []string{"--help"}},
		{name: "daemon help", run: runDaemon, args: []string{"help"}},
		{name: "daemon install help", run: runDaemonInstall, args: []string{"--help"}},
		{name: "daemon uninstall help", run: runDaemonUninstall, args: []string{"--help"}},
		{name: "delete help", run: runDelete, args: []string{"--help"}, want: 2},
		{name: "restore help", run: runRestore, args: []string{"--help"}, want: 2},
		{name: "tags help", run: runTags, args: []string{"help"}},
		{name: "tags list help", run: runTagsList, args: []string{"--help"}},
		{name: "tags create help", run: runTagsCreate, args: []string{"--help"}, want: 1},
		{name: "tags rename usage", run: runTagsRename, args: nil, want: 2},
		{name: "tags update help", run: runTagsUpdate, args: []string{"--help"}, want: 1},
		{name: "tags delete usage", run: runTagsDelete, args: nil, want: 2},
		{name: "tags add article usage", run: runTagsAddArticle, args: nil, want: 2},
		{name: "tags remove article usage", run: runTagsRemoveArticle, args: nil, want: 2},
		{name: "person identities help", run: runPersonIdentities, args: []string{"help"}},
		{name: "person refresh avatar usage", run: runPersonIdentitiesRefreshAvatar, args: nil, want: 2},
		{name: "person refresh avatars help", run: runPersonIdentitiesRefreshAvatars, args: []string{"--help"}},
		{name: "person list help", run: runPersonIdentitiesList, args: []string{"--help"}},
		{name: "person show usage", run: runPersonIdentitiesShow, args: nil, want: 2},
		{name: "translate help", run: runTranslate, args: []string{"--help"}, want: 2},
		{name: "update help", run: runUpdate, args: []string{"--help"}, want: 2},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			want := tt.want
			got := tt.run(tt.args)
			if got != want {
				t.Fatalf("%s returned %d, want %d", tt.name, got, want)
			}
		})
	}
}

func TestRunCommandsReportConfigFailureBeforeDatabaseIO(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("ENVIRONMENT", "test")
	t.Setenv("SESSION_COOKIE_NAME", "scoop_session")

	cases := []struct {
		name string
		run  func([]string) int
		args []string
		want int
	}{
		{name: "stories", run: runStories, args: []string{"--collection", "openclaw"}, want: 1},
		{name: "stats", run: runStats, args: nil, want: 1},
		{name: "story detail", run: runStoryDetail, args: []string{"story-uuid"}, want: 1},
		{name: "collections", run: runCollections, args: nil, want: 1},
		{name: "search", run: runSearch, args: []string{"--query", "openclaw"}, want: 1},
		{name: "articles list", run: runArticlesList, args: []string{"--collection", "openclaw"}, want: 1},
		{name: "articles add person", run: runArticlesAddPerson, args: []string{"article-uuid", "id://github/handle/octocat"}, want: 1},
		{name: "articles remove person", run: runArticlesRemovePerson, args: []string{"article-uuid", "id://github/handle/octocat"}, want: 1},
		{name: "articles list people", run: runArticlesListPeople, args: []string{"article-uuid"}, want: 1},
		{name: "digest", run: runDigest, args: []string{"--collection", "openclaw"}, want: 1},
		{name: "normalize", run: runNormalize, args: []string{"--limit", "1"}, want: 1},
		{name: "embed", run: runEmbed, args: []string{"--limit", "1"}, want: 1},
		{name: "dedup", run: runDedup, args: []string{"--limit", "1"}, want: 1},
		{name: "process", run: runProcess, args: []string{"--until-empty=false"}, want: 1},
		{name: "delete story", run: runDelete, args: []string{"story", "--force", "story-uuid"}, want: 1},
		{name: "restore story", run: runRestore, args: []string{"story", "--force", "story-uuid"}, want: 1},
		{name: "translate story", run: runTranslate, args: []string{"story", "--lang", "zh", "story-uuid"}, want: 1},
		{name: "tags list", run: runTagsList, args: nil, want: 1},
		{name: "tags delete", run: runTagsDelete, args: []string{"i0"}, want: 1},
		{name: "person identities list", run: runPersonIdentitiesList, args: nil, want: 1},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.run(tt.args); got != tt.want {
				t.Fatalf("%s returned %d, want %d", tt.name, got, tt.want)
			}
		})
	}
}

func TestRunReadCommandsReportDatabaseOpenFailure(t *testing.T) {
	t.Setenv("DATABASE_URL", "://bad-url")
	t.Setenv("ENVIRONMENT", "test")
	t.Setenv("LOG_LEVEL", "error")
	t.Setenv("SESSION_COOKIE_NAME", "scoop_session")
	t.Setenv("SESSION_TTL_HOURS", "1")

	cases := []struct {
		name string
		run  func([]string) int
		args []string
	}{
		{name: "stats", run: runStats, args: []string{"--timeout", "1ms"}},
		{name: "collections", run: runCollections, args: []string{"--timeout", "1ms"}},
		{name: "search", run: runSearch, args: []string{"--timeout", "1ms", "--query", "openclaw"}},
		{name: "articles", run: runArticlesList, args: []string{"--timeout", "1ms"}},
		{name: "tags", run: runTagsList, args: []string{"--timeout", "1ms"}},
		{name: "person identities", run: runPersonIdentitiesList, args: []string{"--timeout", "1ms"}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.run(tt.args); got != 1 {
				t.Fatalf("%s returned %d, want database open failure", tt.name, got)
			}
		})
	}
}
