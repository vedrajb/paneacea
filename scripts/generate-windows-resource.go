package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/tc-hib/winres"
)

func main() {
	iconPath := flag.String("icon", "", "path to the ICO file")
	outputPath := flag.String("out", "", "path to the generated Windows resource")
	flag.Parse()
	if *iconPath == "" || *outputPath == "" {
		fmt.Fprintln(os.Stderr, "usage: generate-windows-resource -icon path -out path")
		os.Exit(2)
	}
	iconFile, err := os.Open(*iconPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer iconFile.Close()
	icon, err := winres.LoadICO(iconFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	resources := winres.ResourceSet{}
	if err := resources.SetIcon(winres.RT_ICON, icon); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	outputFile, err := os.Create(*outputPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer outputFile.Close()
	if err := resources.WriteObject(outputFile, winres.ArchAMD64); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
