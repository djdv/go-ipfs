package mountinter

import (
	"bufio"
	"os"
)

func init() {
	suggestedProvider = linuxProviderCheck
}

func linuxProviderCheck() ProviderType {
	pFile, err := os.Open("/proc/filesystems")
	if err != nil {
		return ProviderFuse // NOTE: this is a fallback since the kernel didn't tell us what it supports
	}
	defer pFile.Close()

	wordScanner := bufio.NewScanner(pFile)
	wordScanner.Split(bufio.ScanWords)

	var hasFuse bool
	for wordScanner.Scan() {
		switch wordScanner.Text() {
		case "9p": // This is our preferred provider for this platform. If support is found, don't bother checking anything else.
			return ProviderPlan9Protocol

		case "fuse":
			hasFuse = true
		}
	}

	if hasFuse {
		return ProviderFuse
	}
	return ProviderNone
}
