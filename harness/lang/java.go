package lang

import (
	"context"
	"time"
)

// java scaffolds a Spring Boot project by downloading a starter from
// start.spring.io (the canonical generator) and unzipping it in place.
type java struct{}

func (java) Lang() string        { return "java" }
func (java) Toolchain() []string { return []string{"curl", "unzip"} }

func (java) Frameworks() []Framework {
	return []Framework{
		{ID: "spring-boot", Priority: 40, Keywords: kw(`spring ?boot|\bspring\b`), Markers: []Marker{
			{File: "pom.xml", Contains: "spring-boot"}, {File: "build.gradle", Contains: "org.springframework.boot"},
		}},
	}
}

func (java) Scaffold(ctx context.Context, r Req) (Result, error) {
	res, err := javaRecipe().run(ctx, r)
	if err == nil {
		res.Lang = "java"
	}
	return res, err
}

// Install is a no-op: Maven/Gradle resolve dependencies from the build file, not
// an imperative add command. Editing pom.xml/build.gradle is the model's job.
func (java) Install(ctx context.Context, workDir string, pkgs []string) error { return nil }

func javaRecipe() Recipe {
	// start.spring.io serves a ready Maven project as a zip whose files sit at the
	// archive root, so unzip -d . lays a flat project with no nesting.
	return Recipe{
		Framework: "spring-boot", Requires: []string{"curl", "unzip"}, Timeout: 240 * time.Second,
		Project: "demo", App: "demo", Settings: "src/main/resources/application.properties",
		Stack: "Spring Boot (Java, Maven)", Entry: "./mvnw spring-boot:run",
		Install: []Cmd{
			{Bin: "curl", Args: []string{"-fsSL", "https://start.spring.io/starter.zip",
				"-d", "type=maven-project", "-d", "language=java", "-d", "dependencies=web",
				"-d", "name={{.project}}", "-o", ".harness_starter.zip"}},
			{Bin: "unzip", Args: []string{"-q", "-o", ".harness_starter.zip", "-d", "."}},
			{Bin: "rm", Args: []string{"-f", ".harness_starter.zip"}},
		},
		Verify: "pom.xml",
		Post: []Post{
			{Render: "java/application.properties.tmpl", To: "src/main/resources/application.properties", Append: true, When: "has:postgres"},
		},
		Layout: []string{"pom.xml", "src/main/resources/application.properties"},
	}
}
