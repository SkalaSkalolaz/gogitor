package prompts

import (
	"fmt"
	"strings"

	"gogitor/internal/textutil"
)

// ArticleSimple генерирует промпт для короткой заметки/поста.
// Оптимизирован для моделей 4B–31B: минимум инструкций, чёткая структура.
func ArticleSimple(topic, projectContext, lang string, genre string) string {
	var b strings.Builder
	b.WriteString("You are a technical writer. Write a short text.\n\n")
	b.WriteString("GENRE: " + genre + "\n")
	b.WriteString("TOPIC: " + topic + "\n")

	if strings.TrimSpace(projectContext) != "" {
		b.WriteString("\nPROJECT CONTEXT (use only if relevant):\n")
		b.WriteString(textutil.TruncateStringBytes(projectContext, 4000))
		b.WriteString("\n")
	}

	b.WriteString("\nRULES:\n")
	b.WriteString("1. Write in " + lang + ".\n")
	b.WriteString("2. Format: GitHub Flavored Markdown.\n")
	b.WriteString("3. Structure: title (H1), introduction, main body, conclusion.\n")
	b.WriteString("4. Total length: 300-800 words.\n")
	b.WriteString("5. Include code examples in fenced blocks only for technical topics.\n")
	b.WriteString("6. Do not invent facts. Do not add disclaimers.\n")
	b.WriteString("7. Do not return --- File: blocks.\n")

	if genre == "story" {
		b.WriteString("8. Write in a narrative style with a beginning, middle, and end.\n")
	} else if genre == "news" {
		b.WriteString("8. Use journalistic style: inverted pyramid, facts first.\n")
	}

	return b.String()
}

// ArticlePlan генерирует промпт для создания плана сложной статьи.
func ArticlePlan(topic, projectContext, lang string, genre string, maxSections int) string {
	if maxSections <= 0 {
		maxSections = 5
	}
	if maxSections > 7 {
		maxSections = 7
	}

	var b strings.Builder
	b.WriteString("You are a technical writer planning an article.\n")
	b.WriteString("Return ONLY valid compact JSON. No markdown. No explanations.\n\n")
	b.WriteString("JSON schema:\n")
	b.WriteString(`{"title":"article title","sections":[{"heading":"section heading","key_points":["point 1","point 2"]}]}`)
	b.WriteString("\n\nRULES:\n")
	b.WriteString(fmt.Sprintf("1. Maximum %d sections.\n", maxSections))
	b.WriteString("2. Each section has 2-3 key points.\n")
	b.WriteString("3. Write in " + lang + ".\n")
	b.WriteString("4. Genre: " + genre + ".\n")

	if genre == "story" {
		b.WriteString("5. Use narrative structure: setup, development, climax, resolution.\n")
	} else {
		b.WriteString("5. Sections must cover the topic logically: introduction, main content, conclusion.\n")
	}

	b.WriteString("\nTOPIC: " + topic + "\n")

	if strings.TrimSpace(projectContext) != "" {
		b.WriteString("\nPROJECT CONTEXT:\n")
		b.WriteString(textutil.TruncateStringBytes(projectContext, 3000))
		b.WriteString("\n")
	}

	return b.String()
}

// ArticleSection генерирует промпт для написания одной секции статьи.
func ArticleSection(
	topic string,
	planTitle string,
	sectionHeading string,
	keyPoints []string,
	prevSectionsText string,
	projectContext string,
	lang string,
	genre string,
	isLast bool,
) string {
	var b strings.Builder
	b.WriteString("You are a technical writer. Write ONE section of a text.\n\n")
	b.WriteString("GENRE: " + genre + "\n")
	b.WriteString("ARTICLE TITLE: " + planTitle + "\n")
	b.WriteString("TOPIC: " + topic + "\n\n")
	b.WriteString("CURRENT SECTION: " + sectionHeading + "\n")
	b.WriteString("KEY POINTS TO COVER:\n")
	for _, kp := range keyPoints {
		b.WriteString("- " + kp + "\n")
	}

	if strings.TrimSpace(prevSectionsText) != "" {
		b.WriteString("\nPREVIOUS SECTIONS (for context and continuity):\n")
		b.WriteString(textutil.TruncateStringBytes(prevSectionsText, 6000))
		b.WriteString("\n")
	}

	if strings.TrimSpace(projectContext) != "" {
		b.WriteString("\nPROJECT CONTEXT:\n")
		b.WriteString(textutil.TruncateStringBytes(projectContext, 3000))
		b.WriteString("\n")
	}

	b.WriteString("\nRULES:\n")
	b.WriteString("1. Write in " + lang + ".\n")
	b.WriteString("2. Format: GitHub Flavored Markdown.\n")
	b.WriteString("3. Start with the section heading as H2: ## " + sectionHeading + "\n")
	b.WriteString("4. Length: 150-400 words for this section.\n")
	b.WriteString("5. Do not repeat information from previous sections.\n")

	if genre == "technical" || genre == "howto" || genre == "code_desc" {
		b.WriteString("6. Include code examples in fenced blocks with language tags when relevant.\n")
	}
	if genre == "story" {
		b.WriteString("6. Write in a narrative, engaging style.\n")
	}
	if genre == "news" {
		b.WriteString("6. Use factual, objective language.\n")
	}

	if isLast {
		b.WriteString("7. This is the LAST section. Include a brief conclusion.\n")
	} else {
		b.WriteString("7. Do not write a conclusion.\n")
	}
	b.WriteString("8. Do not return --- File: blocks.\n")

	return b.String()
}

// ArticlePlanFallback — текстовый fallback для малых моделей,
// которые не могут надёжно генерировать JSON.
func ArticlePlanFallback(topic, lang string, maxSections int) string {
	var b strings.Builder
	b.WriteString("You are a technical writer. Create an article outline.\n\n")
	b.WriteString("TOPIC: " + topic + "\n\n")
	b.WriteString("RULES:\n")
	b.WriteString("1. Write in " + lang + ".\n")
	b.WriteString(fmt.Sprintf("2. Return exactly %d lines.\n", maxSections))
	b.WriteString("3. Each line format: Section N: <heading>\n")
	b.WriteString("4. No explanations, no extra text.\n\n")
	b.WriteString("EXAMPLE:\n")
	b.WriteString("Section 1: Introduction to the topic\n")
	b.WriteString("Section 2: Core concepts\n")
	b.WriteString("Section 3: Practical implementation\n")
	return b.String()
}

// ArticleClassify просит LLM определить жанр и параметры статьи.
func ArticleClassify(topic, lang string) string {
	var b strings.Builder
	b.WriteString("You are a task classifier for a text generation system.\n")
	b.WriteString("Return ONLY valid compact JSON. No markdown. No explanations.\n\n")
	b.WriteString("JSON schema:\n")
	b.WriteString(`{"genre":"technical|news|story|review|howto|code_desc|free","need_web_search":true,"need_project_context":false}`)
	b.WriteString("\n\nGENRE RULES:\n")
	b.WriteString("- technical: technical article, code explanation, API docs\n")
	b.WriteString("- news: news, release, announcement, event\n")
	b.WriteString("- story: story, tale, narrative, fiction\n")
	b.WriteString("- review: comparison, overview, vs, benchmark\n")
	b.WriteString("- howto: instruction, guide, tutorial, how to\n")
	b.WriteString("- code_desc: describe code, explain function, what does it do\n")
	b.WriteString("- free: anything else, essay, opinion, creative\n\n")
	b.WriteString("SEARCH RULES:\n")
	b.WriteString("- need_web_search=true for news, review, howto with external tools\n")
	b.WriteString("- need_project_context=true for technical, code_desc, howto about this project\n\n")
	b.WriteString("TOPIC: " + topic + "\n")
	b.WriteString("LANGUAGE: " + lang + "\n")
	return b.String()
}