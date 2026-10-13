package entity

import (
	"net/url"
	"testing"

	"github.com/google/uuid"
)

func TestSeedDataMatchesPrototypeSections(t *testing.T) {
	libraries, documents, sessions := SeedData()
	if len(libraries) != 3 || len(documents) != 15 || len(sessions) != 3 {
		t.Fatalf("演示数据数量不符合预期：libraries=%d documents=%d sessions=%d", len(libraries), len(documents), len(sessions))
	}
	for _, library := range libraries {
		if library.Key == "" || library.Name == "" {
			t.Fatalf("知识库缺少稳定标识或名称：%+v", library)
		}
	}
}

func TestAIArticlesCoverTenDevelopmentTopics(t *testing.T) {
	articles := AIArticles()
	if len(articles) != 10 {
		t.Fatalf("AI 开发文章数量不符合预期：got=%d want=10", len(articles))
	}
	keys := make(map[string]struct{}, len(articles))
	for _, article := range articles {
		if article.Key == "" || article.Name == "" || article.Summary == "" || len(article.Tags) < 2 || len(article.Content) < 300 {
			t.Fatalf("文章缺少可检索的完整元数据：%+v", article)
		}
		if _, exists := keys[article.Key]; exists {
			t.Fatalf("文章 key 重复：%s", article.Key)
		}
		keys[article.Key] = struct{}{}
	}
}

func TestGoTutorialSourcesAreLicensedGitHubHTTPS(t *testing.T) {
	sources := GoTutorialSources()
	if len(sources) < 6 {
		t.Fatalf("Go 教程来源过少：got=%d want>=6", len(sources))
	}
	for _, source := range sources {
		parsed, err := url.Parse(source.RawURL)
		if err != nil || parsed.Scheme != "https" || parsed.Host != "raw.githubusercontent.com" {
			t.Fatalf("抓取地址不在受信任 GitHub 原文主机：%s", source.RawURL)
		}
		if source.SourceURL == "" || source.License != "BSD-3-Clause" || source.Repository != "golang/website" {
			t.Fatalf("来源缺少仓库或许可证信息：%+v", source)
		}
	}
}

func TestIDIsStablePerUserAndKey(t *testing.T) {
	userID := uuid.MustParse("66026edc-ddfa-47b9-8810-1d5a1b3c55b9")
	if got, want := ID(userID, "library:go"), ID(userID, "library:go"); got != want {
		t.Fatalf("相同用户和 key 应得到稳定 ID：got=%s want=%s", got, want)
	}
	if ID(userID, "library:go") == ID(uuid.New(), "library:go") {
		t.Fatal("不同用户不应共享演示数据主键")
	}
}
