package main

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/maxmcd/dag"
)

func TestGetVersions(t *testing.T) {
	brew := NewBrew()

	formulae, err := brew.ListVersions()
	if err != nil {
		t.Fatal(err)
	}
	installed := map[string]struct{}{}
	for _, f := range formulae {
		installed[f.Name] = struct{}{}
	}

	deps, err := brew.DepsGraph("ffmpeg")
	if err != nil {
		t.Fatal(err)
	}
	graph := dag.AcyclicGraph{}

	for _, dep := range deps {
		graph.Add(dep.Source)
		graph.Add(dep.Dependency)
		graph.Connect(dag.BasicEdge(dep.Source, dep.Dependency))
	}

	fmt.Println(string(graph.Dot(nil)))
	// brew list --versions
	// brew info --json ffmpeg
	// brew deps --graph --dot ffmpeg
	// brew outdated

	errs := graph.Walk(func(v dag.Vertex) error {
		formula := v.(string)
		if _, found := installed[formula]; found {
			return nil
		}
		fmt.Println("Installing", formula)
		cmd := exec.Command("brew", "install", formula)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	})
	fmt.Println(errs)
}

/*

Brew install foo

Finds the formula
Installs all dependencies that are needed including existing deps that are out of date
Completes installation

Updates any dependencies that have had their dependencies change as a result of this install
*/
