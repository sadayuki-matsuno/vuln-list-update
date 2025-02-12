package ubuntu

import (
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aquasecurity/vuln-list-update/git"

	"github.com/araddon/dateparse"

	"golang.org/x/xerrors"

	"github.com/aquasecurity/vuln-list-update/utils"
)

const (
	repoURL   = "https://git.launchpad.net/ubuntu-cve-tracker"
	ubuntuDir = "ubuntu"
)

type Vulnerability struct {
	PublicDateAtUSN   time.Time
	CRD               time.Time
	Candidate         string
	PublicDate        time.Time
	References        []string
	Description       string
	UbuntuDescription string
	Notes             []string
	Bugs              []string
	Priority          string
	DiscoveredBy      string
	AssignedTo        string
	Patches           map[Package]Statuses
	UpstreamLinks     map[Package][]string
}

type Package string

type Release string

type Statuses map[Release]Status

type Status struct {
	Status string
	Note   string
}

func Update() error {
	dir := filepath.Join(utils.CacheDir(), "ubuntu-cve-tracker")
	if _, err := git.CloneOrPull(repoURL, dir); err != nil {
		return xerrors.Errorf("failed to clone or pull: %w", err)
	}

	log.Println("Walking Debian...")
	for _, target := range []string{"active", "retired"} {
		if err := walkDir(filepath.Join(dir, target)); err != nil {
			return err
		}
	}

	return nil
}

func walkDir(root string) error {
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}

		base := filepath.Base(path)
		if !strings.HasPrefix(base, "CVE-") {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return xerrors.Errorf("error in file open: %w", err)
		}

		vuln, err := parse(f)
		if err != nil {
			return xerrors.Errorf("error in parse: %w", err)
		}

		if err = utils.SaveCVEPerYear(ubuntuDir, vuln.Candidate, vuln); err != nil {
			return xerrors.Errorf("error in save: %w", err)
		}

		return nil
	})

	if err != nil {
		return xerrors.Errorf("error in walk: %w", err)
	}
	return nil
}

func parse(r io.Reader) (vuln *Vulnerability, err error) {
	vuln = &Vulnerability{}
	vuln.Patches = map[Package]Statuses{}
	vuln.UpstreamLinks = map[Package][]string{}

	all, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(all), "\n")

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// Skip
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		// Parse PublicDateAtUSN
		if strings.HasPrefix(line, "PublicDateAtUSN:") {
			line = strings.TrimPrefix(line, "PublicDateAtUSN:")
			line = strings.TrimSpace(line)
			vuln.PublicDateAtUSN, _ = dateparse.ParseAny(line)
			continue
		}

		// Parse CRD
		if strings.HasPrefix(line, "CRD:") {
			line = strings.TrimPrefix(line, "CRD:")
			line = strings.TrimSpace(line)
			vuln.CRD, _ = dateparse.ParseAny(line)
			continue
		}

		// Parse Candidate
		if strings.HasPrefix(line, "Candidate:") {
			line = strings.TrimPrefix(line, "Candidate:")
			vuln.Candidate = strings.TrimSpace(line)
			continue
		}

		// Parse PublicDate
		if strings.HasPrefix(line, "PublicDate:") {
			line = strings.TrimPrefix(line, "PublicDate:")
			line = strings.TrimSpace(line)
			vuln.PublicDate, _ = dateparse.ParseAny(line)
			continue
		}

		// Parse References
		if strings.HasPrefix(line, "References:") {
			for strings.HasPrefix(lines[i+1], " ") {
				i++
				line = strings.TrimSpace(lines[i])
				vuln.References = append(vuln.References, line)
			}
			continue
		}

		// Parse Description
		if strings.HasPrefix(line, "Description:") {
			var description []string
			for strings.HasPrefix(lines[i+1], " ") {
				i++
				line = strings.TrimSpace(lines[i])
				description = append(description, line)
			}
			vuln.Description = strings.Join(description, " ")
			continue
		}

		// Parse Ubuntu Description
		if strings.HasPrefix(line, "Ubuntu-Description:") {
			var description []string
			for strings.HasPrefix(lines[i+1], " ") {
				i++
				line = strings.TrimSpace(lines[i])
				description = append(description, line)
			}
			vuln.UbuntuDescription = strings.Join(description, " ")
			continue
		}

		// Parse Notes
		if strings.HasPrefix(line, "Notes:") {
			for strings.HasPrefix(lines[i+1], " ") {
				i++
				line = strings.TrimSpace(lines[i])
				note := []string{line}
				for strings.HasPrefix(lines[i+1], "  ") {
					i++
					l := strings.TrimSpace(lines[i])
					note = append(note, l)
				}
				vuln.Notes = append(vuln.Notes, strings.Join(note, " "))
			}
			continue
		}

		// Parse Bugs
		if strings.HasPrefix(line, "Bugs:") {
			for strings.HasPrefix(lines[i+1], " ") {
				i++
				line = strings.TrimSpace(lines[i])
				vuln.Bugs = append(vuln.Bugs, line)
			}
			continue
		}

		// Parse Priority
		if strings.HasPrefix(line, "Priority:") {
			line = strings.TrimPrefix(line, "Priority:")
			vuln.Priority = strings.TrimSpace(line)
			continue
		}

		// Parse Discovered-by
		if strings.HasPrefix(line, "Discovered-by:") {
			line = strings.TrimPrefix(line, "Discovered-by:")
			vuln.DiscoveredBy = strings.TrimSpace(line)
			continue
		}

		// Parse Assigned-to
		if strings.HasPrefix(line, "Assigned-to:") {
			line = strings.TrimPrefix(line, "Assigned-to:")
			vuln.AssignedTo = strings.TrimSpace(line)
			continue
		}

		// Parse Patches
		if strings.HasPrefix(line, "Patches_") {
			suffix := strings.TrimPrefix(line, "Patches")
			statuses := Statuses{}
			var upstreamLinks []string
			for lines[i+1] != "" {
				i++
				line = strings.TrimSpace(lines[i])

				if strings.HasPrefix(line, "upstream:") {
					line = strings.TrimPrefix(line, "upstream:")
					upstreamLinks = append(upstreamLinks, strings.TrimSpace(line))
					continue
				}

				fields := strings.Fields(line)

				if len(fields) < 2 {
					continue
				}

				status := Status{
					Status: fields[1],
				}
				if len(fields) > 2 {
					note := strings.Join(fields[2:], " ")
					status.Note = strings.Trim(note, "()")
				}
				release := Release(strings.TrimSuffix(fields[0], suffix))
				statuses[release] = status
			}
			pkg := Package(strings.Trim(suffix, "_: "))
			vuln.Patches[pkg] = statuses
			if len(upstreamLinks) > 0 {
				vuln.UpstreamLinks[pkg] = upstreamLinks
			}
			continue
		}
	}
	return vuln, nil
}
