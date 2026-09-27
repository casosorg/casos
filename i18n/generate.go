// Copyright 2023 The Casos Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package i18n

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/casosorg/casos/util"
)

type I18nData map[string]map[string]string

var (
	reI18nFrontendNamespace *regexp.Regexp
	reI18nFrontendString    *regexp.Regexp
	reI18nFrontendKey       *regexp.Regexp
	reI18nBackendObject     *regexp.Regexp
	reI18nBackendController *regexp.Regexp
)

func init() {
	// A key the extractor misses never lands in the locale files, and i18next
	// then renders the key itself, so English looks correct and only the other
	// language is visibly broken. Keys reach i18next as translate call
	// arguments, but also as object values translated later (nav.js,
	// helmCompatibilityErrors.js) and as array elements (the example prompts in
	// AgentAccessPage.jsx), so every string literal of the key shape counts.
	// Nothing about "word:word" tells a key apart from a Tailwind class or a URL,
	// so only the namespaces translate calls use are accepted, and parseAllWords
	// applies that filter.
	//
	// The namespace pattern reads the first string inside t(...), which covers
	// t(cond ? "a:x" : "b:y"). Requiring a word boundary before the t keeps
	// split(...) and its kin out.
	reI18nFrontendNamespace = regexp.MustCompile(`\bt\([^()"]*"([A-Za-z][A-Za-z0-9]*):`)
	reI18nFrontendString = regexp.MustCompile(`"((?:[^"\\\n]|\\.)*)"`)
	reI18nFrontendKey = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*:\S`)
	reI18nBackendObject, _ = regexp.Compile("i18n.Translate\\((.*?)\"\\)")
	reI18nBackendController, _ = regexp.Compile("c.T\\((.*?)\"\\)")
}

func getAllI18nNamespacesFrontend(fileContent string) []string {
	res := []string{}
	for _, match := range reI18nFrontendNamespace.FindAllStringSubmatch(fileContent, -1) {
		res = append(res, match[1])
	}
	return res
}

func getAllI18nStringsFrontend(fileContent string) []string {
	res := []string{}
	for _, match := range reI18nFrontendString.FindAllStringSubmatch(fileContent, -1) {
		if isFrontendKey(match[1]) {
			res = append(res, match[1])
		}
	}
	return res
}

// An image reference such as "node:24" has the key shape and a real namespace;
// a key is English text, so it has at least one letter after the colon.
func isFrontendKey(s string) bool {
	if !reI18nFrontendKey.MatchString(s) {
		return false
	}
	return strings.ContainsFunc(strings.SplitN(s, ":", 2)[1], unicode.IsLetter)
}

func getNamespace(word string) string {
	return strings.SplitN(word, ":", 2)[0]
}

func getAllI18nStringsBackend(fileContent string, isControllerPackage bool) []string {
	res := []string{}
	if isControllerPackage {
		matches := reI18nBackendController.FindAllStringSubmatch(fileContent, -1)
		if matches == nil {
			return res
		}
		for _, match := range matches {
			res = append(res, match[1][1:])
		}
	} else {
		matches := reI18nBackendObject.FindAllStringSubmatch(fileContent, -1)
		if matches == nil {
			return res
		}
		for _, match := range matches {
			match := strings.SplitN(match[1], ",", 2)
			res = append(res, match[1][2:])
		}
	}

	return res
}

func getAllFilePathsInFolder(folder string, fileSuffixes ...string) []string {
	res := []string{}
	err := filepath.Walk(folder,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// Hidden directories include .claude/worktrees, whole checkouts of
			// other branches whose keys must not leak into this one.
			if info.IsDir() && path != folder && (info.Name() == "node_modules" || strings.HasPrefix(info.Name(), ".")) {
				return filepath.SkipDir
			}

			if !hasAnySuffix(info.Name(), fileSuffixes) {
				return nil
			}

			res = append(res, path)
			fmt.Println(path, info.Name())
			return nil
		})
	if err != nil {
		panic(err)
	}

	return res
}

func hasAnySuffix(name string, suffixes []string) bool {
	for _, suffix := range suffixes {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

func parseAllWords(category string) *I18nData {
	var paths []string
	if category == "backend" {
		paths = getAllFilePathsInFolder("../", ".go")
	} else {
		// The frontend keeps its components in .jsx and its API clients and
		// helpers in .js, and i18next.t calls appear in both.
		paths = getAllFilePathsInFolder("../web/src", ".js", ".jsx")
	}

	allWords := []string{}
	candidateWords := []string{}
	namespaces := map[string]bool{}
	for _, path := range paths {
		fileContent := util.ReadStringFromPath(path)

		if category == "backend" {
			if strings.HasSuffix(path, "deduplicate_test.go") {
				continue
			}

			isControllerPackage := strings.Contains(path, "controller")
			allWords = append(allWords, getAllI18nStringsBackend(fileContent, isControllerPackage)...)
		} else {
			// Unit tests never render a key, and their node:test imports are in
			// the node namespace.
			if strings.Contains(filepath.Base(path), ".test.") {
				continue
			}

			for _, namespace := range getAllI18nNamespacesFrontend(fileContent) {
				namespaces[namespace] = true
			}
			candidateWords = append(candidateWords, getAllI18nStringsFrontend(fileContent)...)
		}
	}

	for _, word := range candidateWords {
		if namespaces[getNamespace(word)] {
			allWords = append(allWords, word)
		}
	}

	fmt.Printf("%v\n", allWords)

	data := I18nData{}
	for _, word := range allWords {
		tokens := strings.SplitN(word, ":", 2)
		namespace := tokens[0]
		key := tokens[1]

		if _, ok := data[namespace]; !ok {
			data[namespace] = map[string]string{}
		}
		data[namespace][key] = key
	}

	return &data
}

func copyI18nData(src *I18nData) *I18nData {
	dst := I18nData{}
	for namespace, pairs := range *src {
		dst[namespace] = make(map[string]string)
		for key, value := range pairs {
			dst[namespace][key] = value
		}
	}
	return &dst
}

func applyToOtherLanguage(category string, language string, newData *I18nData) {
	oldData := readI18nFile(category, language)
	println(oldData)

	dataCopy := copyI18nData(newData)
	applyData(dataCopy, oldData)
	writeI18nFile(category, language, dataCopy)
}
