package tydocs

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

const persistedIndexVersion = "v1"
const persistedIndexFilename = "tydocs-index.json"
const functionMappingFilename = "\u51fd\u6570\u6620\u5c04\u8868.json"

type Catalog struct {
	syslabRoot string
	launcher   string
	helpRoot   string
	logger     *log.Logger

	mu    sync.Mutex
	index *Index
}

type Index struct {
	Packages []PackageDocs `json:"packages"`
	Entries  []DocEntry    `json:"entries"`
}

type persistedIndex struct {
	Version string `json:"version"`
	Index
}

type PackageDocs struct {
	Name        string `json:"name"`
	Version     string `json:"version,omitempty"`
	PackagePath string `json:"package_path,omitempty"`
	DocsPath    string `json:"docs_path,omitempty"`
	DocsSource  string `json:"docs_source,omitempty"`
	HasDocs     bool   `json:"has_docs"`
}

type DocEntry struct {
	Package    string   `json:"package"`
	Version    string   `json:"version,omitempty"`
	Title      string   `json:"title"`
	Symbol     string   `json:"symbol,omitempty"`
	Summary    string   `json:"summary,omitempty"`
	Path       string   `json:"path"`
	Format     string   `json:"format"`
	Source     string   `json:"source,omitempty"`
	Aliases    []string `json:"aliases,omitempty"`
	searchRel  string
	searchText string
}

type SearchResult struct {
	Packages []PackageDocs `json:"packages"`
	Matches  []DocEntry    `json:"matches"`
}

type ReadResult struct {
	Package string `json:"package"`
	Version string `json:"version,omitempty"`
	Title   string `json:"title"`
	Symbol  string `json:"symbol,omitempty"`
	Summary string `json:"summary,omitempty"`
	Path    string `json:"path"`
	Format  string `json:"format"`
	Source  string `json:"source,omitempty"`
	Content string `json:"content"`
}

type MatlabSymbolCandidate struct {
	MatlabSymbol string `json:"matlab_symbol"`
	SyslabSymbol string `json:"syslab_symbol"`
	Package      string `json:"package,omitempty"`
	Summary      string `json:"summary,omitempty"`
	DocPath      string `json:"doc_path"`
	Source       string `json:"source,omitempty"`
}

type MatlabSymbolResolution struct {
	MatlabSymbol string                  `json:"matlab_symbol"`
	Candidates   []MatlabSymbolCandidate `json:"candidates"`
}

type ResolveMatlabSymbolsResult struct {
	Resolved   []MatlabSymbolResolution `json:"resolved"`
	Unresolved []string                 `json:"unresolved"`
}

func NewCatalog(syslabRoot string, launcher string, helpRoot string, logger *log.Logger) *Catalog {
	return &Catalog{syslabRoot: syslabRoot, launcher: launcher, helpRoot: helpRoot, logger: logger}
}

func PersistedIndexFilename() string {
	return persistedIndexFilename
}

func BuildAndWriteIndexFromAIAssets(aiAssetsRoot string, outputPath string, logger *log.Logger) (string, *Index, error) {
	root := strings.TrimSpace(aiAssetsRoot)
	if root == "" {
		return "", nil, fmt.Errorf("ai assets root is empty")
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", nil, err
	}

	index, err := buildIndexFromAIAssets(absRoot, logger)
	if err != nil {
		return "", nil, err
	}

	out := strings.TrimSpace(outputPath)
	if out == "" {
		out = filepath.Join(absRoot, persistedIndexFilename)
	} else if !filepath.IsAbs(out) {
		if out, err = filepath.Abs(out); err != nil {
			return "", nil, err
		}
	}

	if err := writePersistedIndexFile(out, index); err != nil {
		return "", nil, err
	}
	return out, index, nil
}

func (c *Catalog) SyslabRoot() string {
	if c == nil {
		return ""
	}
	return c.syslabRoot
}

func (c *Catalog) LauncherPath() string {
	if c == nil {
		return ""
	}
	return c.launcher
}

func (c *Catalog) HelpDocsRoot() string {
	if c == nil {
		return ""
	}
	return c.helpRoot
}

func (c *Catalog) Warmup() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.index != nil {
		return nil
	}

	cached, err := c.loadPersistedIndex()
	if err == nil && cached != nil {
		c.index = cached
		return nil
	}
	if c.logger != nil && err != nil && !errors.Is(err, os.ErrNotExist) {
		c.logger.Printf("Ty docs warmup skipped cached index: %v", err)
	}
	return nil
}

func (c *Catalog) Stats() (int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.index == nil {
		return 0, 0
	}

	packagesWithDocs := 0
	for _, pkg := range c.index.Packages {
		if pkg.HasDocs {
			packagesWithDocs++
		}
	}
	return len(c.index.Packages), packagesWithDocs
}

func (c *Catalog) Search(query, packageName string, maxResults int) (SearchResult, error) {
	if strings.TrimSpace(query) == "" {
		return SearchResult{}, fmt.Errorf("query must be a non-empty string")
	}
	if maxResults <= 0 {
		maxResults = 5
	}

	index, err := c.ensureIndex()
	if err != nil {
		return SearchResult{}, err
	}

	filter := strings.ToLower(strings.TrimSpace(packageName))
	packages := make([]PackageDocs, 0, len(index.Packages))
	for _, pkg := range index.Packages {
		if filter != "" && strings.ToLower(pkg.Name) != filter {
			continue
		}
		packages = append(packages, pkg)
	}

	tokens := queryTokens(query)
	queryLower := strings.ToLower(strings.TrimSpace(query))
	matches := make([]scoredEntry, 0, len(index.Entries))
	for _, entry := range index.Entries {
		if filter != "" && strings.ToLower(entry.Package) != filter {
			continue
		}
		score := scoreEntry(entry, queryLower, tokens)
		if score == 0 {
			continue
		}
		matches = append(matches, scoredEntry{DocEntry: entry, score: score})
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score == matches[j].score {
			return matches[i].Path < matches[j].Path
		}
		return matches[i].score > matches[j].score
	})

	limit := maxResults
	if len(matches) < limit {
		limit = len(matches)
	}

	out := make([]DocEntry, 0, limit)
	for _, match := range matches[:limit] {
		out = append(out, DocEntry{
			Package: match.Package,
			Version: match.Version,
			Title:   match.Title,
			Symbol:  match.Symbol,
			Summary: match.Summary,
			Path:    match.Path,
			Format:  match.Format,
			Source:  match.Source,
		})
	}

	return SearchResult{Packages: packages, Matches: out}, nil
}

func (c *Catalog) Read(path string) (ReadResult, error) {
	index, err := c.ensureIndex()
	if err != nil {
		return ReadResult{}, err
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return ReadResult{}, err
	}
	for _, entry := range index.Entries {
		if !strings.EqualFold(entry.Path, abs) {
			continue
		}
		content, err := readDocText(abs)
		if err != nil {
			return ReadResult{}, err
		}
		return ReadResult{
			Package: entry.Package,
			Version: entry.Version,
			Title:   entry.Title,
			Symbol:  entry.Symbol,
			Summary: entry.Summary,
			Path:    entry.Path,
			Format:  entry.Format,
			Source:  entry.Source,
			Content: content,
		}, nil
	}
	return ReadResult{}, fmt.Errorf("document is not part of the indexed Ty docs: %s", path)
}

func (c *Catalog) ResolveMatlabSymbols(symbols []string, maxResultsPerSymbol int) (ResolveMatlabSymbolsResult, error) {
	if maxResultsPerSymbol <= 0 {
		maxResultsPerSymbol = 3
	}

	index, err := c.ensureIndex()
	if err != nil {
		return ResolveMatlabSymbolsResult{}, err
	}

	resolved := make([]MatlabSymbolResolution, 0, len(symbols))
	unresolved := make([]string, 0)
	seenSymbols := make(map[string]struct{}, len(symbols))

	for _, raw := range symbols {
		symbol := strings.TrimSpace(raw)
		if symbol == "" {
			continue
		}
		normalized := strings.ToLower(symbol)
		if _, ok := seenSymbols[normalized]; ok {
			continue
		}
		seenSymbols[normalized] = struct{}{}

		candidates := resolveMatlabSymbolCandidates(index, symbol, maxResultsPerSymbol)
		if len(candidates) == 0 {
			unresolved = append(unresolved, symbol)
			continue
		}

		resolved = append(resolved, MatlabSymbolResolution{
			MatlabSymbol: symbol,
			Candidates:   candidates,
		})
	}

	sort.Strings(unresolved)
	sort.Slice(resolved, func(i, j int) bool {
		return strings.ToLower(resolved[i].MatlabSymbol) < strings.ToLower(resolved[j].MatlabSymbol)
	})

	return ResolveMatlabSymbolsResult{
		Resolved:   resolved,
		Unresolved: unresolved,
	}, nil
}

func (c *Catalog) ensureIndex() (*Index, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.index != nil {
		return c.index, nil
	}

	if cached, err := c.loadPersistedIndex(); err == nil && cached != nil {
		c.index = cached
		return c.index, nil
	}
	if !c.hasDocSources() {
		c.index = &Index{}
		return c.index, nil
	}

	index, err := buildIndex(c.syslabRoot, c.launcher, c.helpRoot, c.logger)
	if err != nil {
		return nil, err
	}
	c.index = index
	_, _ = c.savePersistedIndex(index, "")
	return c.index, nil
}

type scoredMatlabCandidate struct {
	MatlabSymbolCandidate
	score int
}

func resolveMatlabSymbolCandidates(index *Index, symbol string, limit int) []MatlabSymbolCandidate {
	query := strings.TrimSpace(symbol)
	if index == nil || query == "" {
		return nil
	}

	queryLower := strings.ToLower(query)
	candidates := make([]scoredMatlabCandidate, 0)
	seen := make(map[string]struct{})

	for _, entry := range index.Entries {
		score := scoreMatlabCandidate(entry, queryLower)
		if score == 0 {
			continue
		}

		key := strings.ToLower(entry.Path) + "\x00" + queryLower
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		candidates = append(candidates, scoredMatlabCandidate{
			MatlabSymbolCandidate: MatlabSymbolCandidate{
				MatlabSymbol: query,
				SyslabSymbol: firstNonEmpty(strings.TrimSpace(entry.Symbol), strings.TrimSpace(entry.Title)),
				Package:      entry.Package,
				Summary:      entry.Summary,
				DocPath:      entry.Path,
				Source:       matlabCandidateSource(entry, queryLower),
			},
			score: score,
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score == candidates[j].score {
			if !strings.EqualFold(candidates[i].Package, candidates[j].Package) {
				return strings.ToLower(candidates[i].Package) < strings.ToLower(candidates[j].Package)
			}
			return candidates[i].DocPath < candidates[j].DocPath
		}
		return candidates[i].score > candidates[j].score
	})

	if len(candidates) > limit {
		candidates = candidates[:limit]
	}

	out := make([]MatlabSymbolCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, candidate.MatlabSymbolCandidate)
	}
	return out
}

func scoreMatlabCandidate(entry DocEntry, queryLower string) int {
	score := 0

	for _, alias := range entry.Aliases {
		aliasLower := strings.ToLower(strings.TrimSpace(alias))
		if aliasLower == "" {
			continue
		}
		if aliasLower == queryLower {
			score = max(score, 120)
		} else if strings.Contains(aliasLower, queryLower) || strings.Contains(queryLower, aliasLower) {
			score = max(score, 80)
		}
	}

	symbolLower := strings.ToLower(strings.TrimSpace(entry.Symbol))
	titleLower := strings.ToLower(strings.TrimSpace(entry.Title))
	if symbolLower == queryLower {
		score = max(score, 60)
	} else if symbolLower != "" && strings.Contains(symbolLower, queryLower) {
		score = max(score, 40)
	}
	if titleLower == queryLower {
		score = max(score, 50)
	} else if titleLower != "" && strings.Contains(titleLower, queryLower) {
		score = max(score, 30)
	}

	if strings.EqualFold(entry.Source, "function_mapping") {
		score += 20
	}

	return score
}

func matlabCandidateSource(entry DocEntry, queryLower string) string {
	if strings.EqualFold(entry.Source, "function_mapping") {
		return "function_mapping"
	}
	symbolLower := strings.ToLower(strings.TrimSpace(entry.Symbol))
	titleLower := strings.ToLower(strings.TrimSpace(entry.Title))
	for _, alias := range entry.Aliases {
		aliasLower := strings.ToLower(strings.TrimSpace(alias))
		if aliasLower == "" || aliasLower != queryLower {
			continue
		}
		if aliasLower != symbolLower && aliasLower != titleLower {
			return "function_mapping"
		}
	}
	return entry.Source
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (c *Catalog) loadPersistedIndex() (*Index, error) {
	path, err := c.persistedIndexPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var persisted persistedIndex
	if err := json.Unmarshal(data, &persisted); err != nil {
		return nil, err
	}
	if persisted.Version != persistedIndexVersion {
		return nil, fmt.Errorf("unsupported Ty docs index version: %s", persisted.Version)
	}
	if len(persisted.Packages) == 0 {
		return nil, fmt.Errorf("empty Ty docs index")
	}
	resolvePersistedIndexPaths(filepath.Dir(path), &persisted.Index)
	for i := range persisted.Entries {
		hydrateSearchFields(&persisted.Entries[i])
	}
	return &persisted.Index, nil
}

func (c *Catalog) savePersistedIndex(index *Index, outputPath string) (string, error) {
	path := strings.TrimSpace(outputPath)
	var err error
	if path == "" {
		path, err = c.persistedIndexPath()
	}
	if err != nil {
		return "", err
	}
	return path, writePersistedIndexFile(path, index)
}

func (c *Catalog) persistedIndexPath() (string, error) {
	aiAssetsRoot := aiAssetsRootFromSyslabRoot(c.syslabRoot)
	if strings.TrimSpace(c.syslabRoot) == "" {
		return "", fmt.Errorf("syslab root is empty")
	}
	return filepath.Join(aiAssetsRoot, persistedIndexFilename), nil
}

func (c *Catalog) hasDocSources() bool {
	projectsRoot := strings.TrimSpace(c.helpRoot)
	if projectsRoot == "" {
		projectsRoot = defaultProjectsRoot(c.syslabRoot)
	}
	if info, err := os.Stat(projectsRoot); err == nil && info.IsDir() {
		return true
	}

	functionTablePath := functionTablePathFromAIAssets(aiAssetsRootFromSyslabRoot(c.syslabRoot))
	if info, err := os.Stat(functionTablePath); err == nil && !info.IsDir() {
		return true
	}

	return false
}

func defaultProjectsRoot(syslabRoot string) string {
	return projectsRootFromAIAssets(aiAssetsRootFromSyslabRoot(syslabRoot))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func buildIndex(syslabRoot string, _ string, helpRoot string, logger *log.Logger) (*Index, error) {
	if strings.TrimSpace(helpRoot) == "" {
		return buildIndexFromAIAssets(aiAssetsRootFromSyslabRoot(syslabRoot), logger)
	}

	packagesByName := make(map[string]PackageDocs)
	for _, pkg := range discoverProjectPackages(syslabRoot, helpRoot) {
		packagesByName[pkg.Name] = pkg
	}

	packages := make([]PackageDocs, 0, len(packagesByName))
	entries := make([]DocEntry, 0)
	for _, pkgDocs := range packagesByName {
		packages = append(packages, pkgDocs)
		if pkgDocs.DocsPath == "" {
			continue
		}
		docEntries, err := indexPackageDocs(pkgDocs)
		if err != nil {
			if logger != nil {
				logger.Printf("skip Ty docs for %s: %v", pkgDocs.Name, err)
			}
			continue
		}
		entries = append(entries, docEntries...)
	}
	entries = applyFunctionMappings(entries, syslabRoot, helpRoot, logger)

	sort.Slice(packages, func(i, j int) bool { return packages[i].Name < packages[j].Name })
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Package == entries[j].Package {
			return entries[i].Path < entries[j].Path
		}
		return entries[i].Package < entries[j].Package
	})
	return &Index{Packages: packages, Entries: entries}, nil
}

func buildIndexFromAIAssets(aiAssetsRoot string, logger *log.Logger) (*Index, error) {
	packagesByName := make(map[string]PackageDocs)
	projectsRoot := projectsRootFromAIAssets(aiAssetsRoot)
	for _, pkg := range discoverProjectPackagesFromRoot(projectsRoot, "syslab_aiassets") {
		packagesByName[pkg.Name] = pkg
	}

	packages := make([]PackageDocs, 0, len(packagesByName))
	entries := make([]DocEntry, 0)
	for _, pkgDocs := range packagesByName {
		packages = append(packages, pkgDocs)
		if pkgDocs.DocsPath == "" {
			continue
		}
		docEntries, err := indexPackageDocs(pkgDocs)
		if err != nil {
			if logger != nil {
				logger.Printf("skip Ty docs for %s: %v", pkgDocs.Name, err)
			}
			continue
		}
		entries = append(entries, docEntries...)
	}
	entries = applyFunctionMappingsFromPath(entries, functionTablePathFromAIAssets(aiAssetsRoot), projectsRoot, logger)

	sort.Slice(packages, func(i, j int) bool { return packages[i].Name < packages[j].Name })
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Package == entries[j].Package {
			return entries[i].Path < entries[j].Path
		}
		return entries[i].Package < entries[j].Package
	})
	return &Index{Packages: packages, Entries: entries}, nil
}

func discoverProjectPackages(syslabRoot string, helpRoot string) []PackageDocs {
	projectsRoot := strings.TrimSpace(helpRoot)
	if projectsRoot == "" {
		projectsRoot = defaultProjectsRoot(syslabRoot)
	}
	source := "syslab_aiassets"
	if strings.TrimSpace(helpRoot) != "" {
		source = "configured_helpdocs"
	}
	return discoverProjectPackagesFromRoot(projectsRoot, source)
}

func discoverProjectPackagesFromRoot(projectsRoot string, source string) []PackageDocs {
	dirs, err := os.ReadDir(projectsRoot)
	if err != nil {
		return nil
	}
	packages := make([]PackageDocs, 0, len(dirs))
	for _, dir := range dirs {
		if !dir.IsDir() {
			continue
		}
		name := dir.Name()
		docsPath := ""
		for _, candidate := range []string{
			filepath.Join(projectsRoot, name, "Doc"),
			filepath.Join(projectsRoot, name, "doc"),
			filepath.Join(projectsRoot, name),
		} {
			if isDir(candidate) {
				docsPath = candidate
				break
			}
		}
		packages = append(packages, PackageDocs{
			Name:       name,
			DocsPath:   docsPath,
			DocsSource: source,
			HasDocs:    docsPath != "",
		})
	}
	return packages
}

func locateDocsPath(syslabRoot string, helpRoot string, pkg PackageInfo) (string, string) {
	if pkg.PackagePath != "" {
		for _, candidate := range []string{
			filepath.Join(pkg.PackagePath, "docs"),
			filepath.Join(pkg.PackagePath, "doc"),
		} {
			if isDir(candidate) {
				return candidate, "package_docs"
			}
		}
	}

	for _, candidate := range syslabProjectDocCandidates(syslabRoot, helpRoot, pkg.Name) {
		if isDir(candidate) {
			if strings.TrimSpace(helpRoot) != "" {
				return candidate, "configured_helpdocs"
			}
			return candidate, "syslab_aiassets"
		}
	}
	return "", ""
}

func syslabProjectDocCandidates(syslabRoot string, helpRoot string, pkgName string) []string {
	projectsRoot := strings.TrimSpace(helpRoot)
	if projectsRoot == "" {
		projectsRoot = defaultProjectsRoot(syslabRoot)
	}
	return []string{
		filepath.Join(projectsRoot, pkgName, "Doc"),
		filepath.Join(projectsRoot, pkgName, "doc"),
		filepath.Join(projectsRoot, pkgName),
	}
}

func indexPackageDocs(pkg PackageDocs) ([]DocEntry, error) {
	entries := make([]DocEntry, 0)
	err := filepath.WalkDir(pkg.DocsPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() || !isDocFile(path) {
			return nil
		}

		abs, err := filepath.Abs(path)
		if err != nil {
			return nil
		}
		text, err := readDocText(abs)
		if err != nil {
			return nil
		}
		title := extractTitle(text, abs)
		summary := extractSummary(text, title)
		rel, _ := filepath.Rel(pkg.DocsPath, abs)
		entries = append(entries, DocEntry{
			Package:    pkg.Name,
			Version:    pkg.Version,
			Title:      title,
			Symbol:     inferSymbol(abs, title),
			Summary:    summary,
			Path:       abs,
			Format:     strings.TrimPrefix(strings.ToLower(filepath.Ext(abs)), "."),
			Source:     pkg.DocsSource,
			searchRel:  strings.ToLower(rel),
			searchText: strings.ToLower(strings.Join([]string{pkg.Name, title, summary, text}, "\n")),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	for i := range entries {
		hydrateSearchFields(&entries[i])
	}
	return entries, nil
}

type functionMapping struct {
	Package        string `json:"package"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	HelpURL        string `json:"helpUrl"`
	Kind           string `json:"kind"`
	MatlabFunction string `json:"matlabFunction"`
}

func applyFunctionMappings(entries []DocEntry, syslabRoot string, helpRoot string, logger *log.Logger) []DocEntry {
	mappings, err := loadFunctionMappings(syslabRoot)
	if err != nil {
		if logger != nil {
			logger.Printf("skip function mappings: %v", err)
		}
		return entries
	}

	byPath := make(map[string]int, len(entries))
	for i := range entries {
		byPath[strings.ToLower(entries[i].Path)] = i
	}

	for _, item := range mappings {
		resolvedPath := resolveFunctionMappingDocPath(syslabRoot, helpRoot, item)
		if resolvedPath == "" {
			continue
		}
		aliases := uniqueStrings(item.Name, item.MatlabFunction)
		if idx, ok := byPath[strings.ToLower(resolvedPath)]; ok {
			if entries[idx].Summary == "" && strings.TrimSpace(item.Description) != "" {
				entries[idx].Summary = strings.TrimSpace(item.Description)
			}
			entries[idx].Aliases = uniqueStrings(append(entries[idx].Aliases, aliases...)...)
			hydrateSearchFields(&entries[idx])
			continue
		}

		text, err := readDocText(resolvedPath)
		if err != nil {
			continue
		}
		title := extractTitle(text, resolvedPath)
		summary := extractSummary(text, title)
		if summary == "" {
			summary = strings.TrimSpace(item.Description)
		}
		entry := DocEntry{
			Package: item.Package,
			Title:   title,
			Symbol:  firstNonEmpty(strings.TrimSpace(item.Name), inferSymbol(resolvedPath, title)),
			Summary: summary,
			Path:    resolvedPath,
			Format:  strings.TrimPrefix(strings.ToLower(filepath.Ext(resolvedPath)), "."),
			Source:  "function_mapping",
			Aliases: aliases,
		}
		hydrateSearchFields(&entry)
		byPath[strings.ToLower(entry.Path)] = len(entries)
		entries = append(entries, entry)
	}
	return entries
}

func loadFunctionMappings(syslabRoot string) ([]functionMapping, error) {
	path := functionTablePathFromAIAssets(aiAssetsRootFromSyslabRoot(syslabRoot))
	return loadFunctionMappingsFromPath(path)
}

func loadFunctionMappingsFromPath(path string) ([]functionMapping, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var mappings []functionMapping
	if err := json.Unmarshal(data, &mappings); err != nil {
		return nil, err
	}
	return mappings, nil
}

func resolveFunctionMappingDocPath(syslabRoot string, helpRoot string, item functionMapping) string {
	projectsRoot := strings.TrimSpace(helpRoot)
	if projectsRoot == "" {
		projectsRoot = defaultProjectsRoot(syslabRoot)
	}
	return resolveFunctionMappingDocPathFromProjectsRoot(projectsRoot, item)
}

func resolveFunctionMappingDocPathFromProjectsRoot(projectsRoot string, item functionMapping) string {
	helpURL := strings.TrimSpace(item.HelpURL)
	if helpURL == "" || strings.TrimSpace(item.Package) == "" {
		return ""
	}

	relative := strings.TrimPrefix(helpURL, "/")
	relative = filepath.FromSlash(relative)
	candidates := []string{
		filepath.Join(projectsRoot, item.Package, relative),
	}
	base := strings.TrimSuffix(candidates[0], filepath.Ext(candidates[0]))
	if filepath.Ext(base) == "" {
		return firstExistingFile(candidates...)
	}
	return firstExistingFile(
		candidates[0],
		base+".md",
		base+".html",
		base+".htm",
		base+".txt",
	)
}

func applyFunctionMappingsFromPath(entries []DocEntry, functionTablePath string, projectsRoot string, logger *log.Logger) []DocEntry {
	mappings, err := loadFunctionMappingsFromPath(functionTablePath)
	if err != nil {
		if logger != nil {
			logger.Printf("skip function mappings: %v", err)
		}
		return entries
	}

	byPath := make(map[string]int, len(entries))
	for i := range entries {
		byPath[strings.ToLower(entries[i].Path)] = i
	}

	for _, item := range mappings {
		resolvedPath := resolveFunctionMappingDocPathFromProjectsRoot(projectsRoot, item)
		if resolvedPath == "" {
			continue
		}
		aliases := uniqueStrings(item.Name, item.MatlabFunction)
		if idx, ok := byPath[strings.ToLower(resolvedPath)]; ok {
			if entries[idx].Summary == "" && strings.TrimSpace(item.Description) != "" {
				entries[idx].Summary = strings.TrimSpace(item.Description)
			}
			entries[idx].Aliases = uniqueStrings(append(entries[idx].Aliases, aliases...)...)
			hydrateSearchFields(&entries[idx])
			continue
		}

		text, err := readDocText(resolvedPath)
		if err != nil {
			continue
		}
		title := extractTitle(text, resolvedPath)
		summary := extractSummary(text, title)
		if summary == "" {
			summary = strings.TrimSpace(item.Description)
		}
		entry := DocEntry{
			Package: item.Package,
			Title:   title,
			Symbol:  firstNonEmpty(strings.TrimSpace(item.Name), inferSymbol(resolvedPath, title)),
			Summary: summary,
			Path:    resolvedPath,
			Format:  strings.TrimPrefix(strings.ToLower(filepath.Ext(resolvedPath)), "."),
			Source:  "function_mapping",
			Aliases: aliases,
		}
		hydrateSearchFields(&entry)
		byPath[strings.ToLower(entry.Path)] = len(entries)
		entries = append(entries, entry)
	}
	return entries
}

func aiAssetsRootFromSyslabRoot(syslabRoot string) string {
	return filepath.Join(syslabRoot, "Tools", "AIAssets")
}

func projectsRootFromAIAssets(aiAssetsRoot string) string {
	for _, candidate := range []string{
		filepath.Join(aiAssetsRoot, "projects"),
		filepath.Join(aiAssetsRoot, "syslabHelpSourceCode", "projects"),
	} {
		if isDir(candidate) {
			return candidate
		}
	}
	return filepath.Join(aiAssetsRoot, "projects")
}

func functionTablePathFromAIAssets(aiAssetsRoot string) string {
	for _, candidate := range []string{
		filepath.Join(aiAssetsRoot, "static", "FunctionTable", functionMappingFilename),
		filepath.Join(aiAssetsRoot, "SearchCenter", "static", "FunctionTable", functionMappingFilename),
	} {
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return candidate
		}
	}
	return filepath.Join(aiAssetsRoot, "static", "FunctionTable", functionMappingFilename)
}

func writePersistedIndexFile(path string, index *Index) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	persisted := persistedIndex{
		Version: persistedIndexVersion,
		Index:   relativePersistedIndex(filepath.Dir(path), index),
	}
	data, err := json.Marshal(&persisted)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func relativePersistedIndex(baseDir string, index *Index) Index {
	if index == nil {
		return Index{}
	}
	baseDir = strings.TrimSpace(baseDir)
	out := Index{
		Packages: make([]PackageDocs, len(index.Packages)),
		Entries:  make([]DocEntry, len(index.Entries)),
	}
	for i, pkg := range index.Packages {
		out.Packages[i] = pkg
		out.Packages[i].DocsPath = relativizeStoredPath(baseDir, pkg.DocsPath)
	}
	for i, entry := range index.Entries {
		out.Entries[i] = entry
		out.Entries[i].Path = relativizeStoredPath(baseDir, entry.Path)
		out.Entries[i].searchRel = ""
		out.Entries[i].searchText = ""
	}
	return out
}

func resolvePersistedIndexPaths(baseDir string, index *Index) {
	if index == nil {
		return
	}
	baseDir = strings.TrimSpace(baseDir)
	for i := range index.Packages {
		index.Packages[i].DocsPath = resolveStoredPath(baseDir, index.Packages[i].DocsPath)
	}
	for i := range index.Entries {
		index.Entries[i].Path = resolveStoredPath(baseDir, index.Entries[i].Path)
	}
}

func relativizeStoredPath(baseDir string, path string) string {
	path = strings.TrimSpace(path)
	baseDir = strings.TrimSpace(baseDir)
	if path == "" || baseDir == "" {
		return filepath.ToSlash(path)
	}
	if !filepath.IsAbs(path) {
		return filepath.ToSlash(path)
	}
	rel, err := filepath.Rel(baseDir, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func resolveStoredPath(baseDir string, path string) string {
	path = strings.TrimSpace(path)
	baseDir = strings.TrimSpace(baseDir)
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) || baseDir == "" {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(baseDir, filepath.FromSlash(path)))
}

func firstExistingFile(paths ...string) string {
	for _, path := range paths {
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() {
			abs, absErr := filepath.Abs(path)
			if absErr == nil {
				return abs
			}
			return path
		}
	}
	return ""
}

func uniqueStrings(values ...string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func hydrateSearchFields(entry *DocEntry) {
	if entry == nil {
		return
	}
	entry.searchRel = strings.ToLower(filepath.ToSlash(entry.Path))
	searchParts := []string{entry.Package, entry.Title, entry.Symbol, entry.Summary, entry.Path}
	searchParts = append(searchParts, entry.Aliases...)
	entry.searchText = strings.ToLower(strings.Join(searchParts, "\n"))
}

type scoredEntry struct {
	DocEntry
	score int
}

func scoreEntry(entry DocEntry, query string, tokens []string) int {
	score := 0
	titleLower := strings.ToLower(entry.Title)
	symbolLower := strings.ToLower(entry.Symbol)
	packageLower := strings.ToLower(entry.Package)

	if query != "" {
		if strings.Contains(titleLower, query) {
			score += 150
		}
		if strings.Contains(symbolLower, query) {
			score += 200
		}
		if strings.Contains(packageLower, query) {
			score += 100
		}
		if strings.Contains(entry.searchRel, query) {
			score += 90
		}
		if strings.Contains(entry.searchText, query) {
			score += 60
		}
	}

	hits := 0
	for _, token := range tokens {
		if strings.Contains(titleLower, token) {
			score += 60
			hits++
		}
		if token != "" && symbolLower == token {
			score += 140
			hits++
		} else if strings.Contains(symbolLower, token) {
			score += 70
			hits++
		}
		if strings.Contains(packageLower, token) {
			score += 50
			hits++
		}
		if strings.Contains(entry.searchText, token) {
			score += 20
			hits++
		}
	}
	if hits >= 2 {
		score += 40
	}
	return score
}

func extractTitle(text, path string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimLeft(line, "#"))
		if len([]rune(line)) >= 2 {
			return line
		}
	}
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func extractSummary(text, title string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.EqualFold(strings.TrimLeft(line, "# "), title) {
			continue
		}
		if len([]rune(line)) > 240 {
			return string([]rune(line)[:240])
		}
		return line
	}
	return ""
}

var symbolPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_!?.]*$`)

func inferSymbol(path, title string) string {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if symbolPattern.MatchString(base) {
		return base
	}
	fields := strings.Fields(title)
	if len(fields) > 0 && symbolPattern.MatchString(fields[0]) {
		return fields[0]
	}
	return ""
}
