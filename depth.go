// Package depth provides the ability to traverse and retrieve Go source code dependencies in the form of
// internal and external packages.
//
// For example, the dependencies of the stdlib `strings` package can be resolved like so:
//
// 	import "github.com/KyleBanks/depth"
//
//	var t depth.Tree
// 	err := t.Resolve("strings")
// 	if err != nil {
// 		log.Fatal(err)
// 	}
//
// 	// Output: "strings has 4 dependencies."
// 	log.Printf("%v has %v dependencies.", t.Root.Name, len(t.Root.Deps))
//
// For additional customization, simply set the appropriate flags on the `Tree` before resolving:
//
// 	import "github.com/KyleBanks/depth"
//
// 	t := depth.Tree {
//  	ResolveInternal: true,
//   	ResolveTest: true,
//   	MaxDepth: 10,
// 	}
// 	err := t.Resolve("strings")
package depth

import (
	"errors"
	"go/build"
	"os"
	"regexp"
)

// ErrRootPkgNotResolved is returned when the root Pkg of the Tree cannot be resolved,
// typically because it does not exist.
var ErrRootPkgNotResolved = errors.New("unable to resolve root package")

// Importer defines a type that can import a package and return its details.
type Importer interface {
	Import(name, srcDir string, im build.ImportMode) (*build.Package, error)
}

// Tree represents the top level of a Pkg and the configuration used to
// initialize and represent its contents.
type Tree struct {
	Root *Pkg

	ResolveInternal bool
	ResolveTest     bool
	MaxDepth        int
	MapLevel        int    // 扁平化展示层级
	ShowPkg         string // 展示哪个包的依赖
	showFiltered    func(pkgName string) bool

	MatcherReg string
	matched    *regexp.Regexp

	Importer Importer

	importCache    map[string]struct{}
	importPkgCache map[string]*build.Package
}

func (t *Tree) ShowFilter(pkgName string) bool {
	if t.showFiltered != nil {
		return t.showFiltered(pkgName)
	}
	return false
}

func (t *Tree) hasSeenPkg(name string) (*build.Package, bool) {
	pkg, exist := t.importPkgCache[name]
	return pkg, exist
}

func (t *Tree) cachePkg(name string, pkg *build.Package) {
	t.importPkgCache[name] = pkg
}

func (t *Tree) Init() {
	if len(t.MatcherReg) > 0 {
		t.matched = regexp.MustCompile(t.MatcherReg)
	}
	if len(t.ShowPkg) > 0 {
		t.showFiltered = func(pkgName string) bool {
			if pkgName != t.ShowPkg {
				return true
			}
			return false
		}
	}
	t.importPkgCache = make(map[string]*build.Package)
}

// Resolve recursively finds all dependencies for the root Pkg name provided,
// and the packages it depends on.
func (t *Tree) Resolve(name string) error {
	pwd, err := os.Getwd()
	if err != nil {
		return err
	}

	t.Root = &Pkg{
		Name:   name,
		Tree:   t,
		SrcDir: pwd,
		Test:   false,
	}

	// Reset the import cache each time to ensure a reused Tree doesn't
	// reuse the same cache.
	t.importCache = nil

	// Allow custom importers, but use build.Default if none is provided.
	if t.Importer == nil {
		t.Importer = &build.Default
	}

	t.Root.Resolve(t.Importer)
	if !t.Root.Resolved {
		return ErrRootPkgNotResolved
	}

	return nil
}

// shouldResolveInternal determines if internal packages should be further resolved beyond the
// current parent.
//
// For example, if the parent Pkg is `github.com/foo/bar` and true is returned, all the
// internal dependencies it relies on will be resolved. If for example `strings` is one of those
// dependencies, and it is passed as the parent here, false may be returned and its internal
// dependencies will not be resolved.
func (t *Tree) shouldResolveInternal(parent *Pkg) bool {
	if t.ResolveInternal {
		return true
	}

	return parent == t.Root
}

func (t *Tree) shouldFiltered(name string) bool {
	if t.matched != nil {
		if t.matched.Match([]byte(name)) {
			return false
		}
		return true
	}
	return false
}

// isAtMaxDepth returns true when the depth of the Pkg provided is at or beyond the maximum
// depth allowed by the tree.
//
// If the Tree has a MaxDepth of zero, true is never returned.
func (t *Tree) isAtMaxDepth(p *Pkg) bool {
	if t.MaxDepth == 0 {
		return false
	}

	return p.depth() >= t.MaxDepth
}

// hasSeenImport returns true if the import name provided has already been seen within the tree.
// This function only returns false for a name once.
func (t *Tree) hasSeenImport(name string) bool {
	if t.importCache == nil {
		t.importCache = make(map[string]struct{})
	}

	if _, ok := t.importCache[name]; ok {
		return true
	}
	t.importCache[name] = struct{}{}
	return false
}
