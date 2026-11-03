package gorest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/mod/semver"
)

const (
	latestGORESTReleaseURL = "https://api.github.com/repos/elsyahtech/gorest/releases/latest"
	gorestReleasePageURL   = "https://github.com/elsyahtech/gorest/releases/tag/"
	updateCheckTimeout     = 3 * time.Second
	maxReleaseResponse     = 1 << 20
)

var errNoPublishedGORESTRelease = errors.New("no published GOREST release found")

type releaseInfo struct {
	TagName string `json:"tag_name"`
}

type updateNotice struct {
	InstalledVersion string
	LatestVersion    string
	ReleaseURL       string
}

func (app *App) checkForGorestUpdates(parent context.Context) {
	if app == nil || !updateChecksEnabled(app.config.CheckForUpdates) {
		return
	}

	app.updateCheckWG.Add(1)
	go func() {
		defer app.updateCheckWG.Done()

		ctx, cancel := context.WithTimeout(parent, updateCheckTimeout)
		defer cancel()

		installedVersion := currentGORESTVersion()
		notice, err := checkForUpdate(ctx, installedVersion, http.DefaultClient, latestGORESTReleaseURL)
		if err != nil {
			if errors.Is(err, errNoPublishedGORESTRelease) {
				app.Log(map[string]any{
					"installed_version": installedVersion,
				}, true).Info("no published GOREST release found; update check skipped")

				return
			}

			app.Log(map[string]any{
				"error":             err,
				"installed_version": installedVersion,
			}, true).Warn("could not check for GOREST updates")
			return
		}

		if notice == nil {
			return
		}

		app.Log(map[string]any{
			"installed_version": notice.InstalledVersion,
			"latest_version":    notice.LatestVersion,
			"release_url":       notice.ReleaseURL,
		}, true).Info("a newer GOREST release is available; upgrade when convenient")
	}()
}

func updateChecksEnabled(enabled *bool) bool {
	return enabled == nil || *enabled
}

func checkForUpdate(
	ctx context.Context,
	installedVersion string,
	client *http.Client,
	releaseURL string,
) (*updateNotice, error) {
	if !semver.IsValid(installedVersion) {
		return nil, nil
	}

	if ctx == nil {
		return nil, errors.New("update check context is nil")
	}

	if client == nil {
		return nil, errors.New("update check HTTP client is nil")
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, releaseURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create GOREST release request: %w", err)
	}

	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "gorest-update-check")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request latest GOREST release: %w", err)
	}

	defer response.Body.Close()

	if response.StatusCode == http.StatusNotFound {
		return nil, errNoPublishedGORESTRelease
	}

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("latest GOREST release request returned HTTP %d", response.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxReleaseResponse+1))
	if err != nil {
		return nil, fmt.Errorf("read latest GOREST release response: %w", err)
	}

	if len(body) > maxReleaseResponse {
		return nil, fmt.Errorf("latest GOREST release response exceeds %d bytes", maxReleaseResponse)
	}

	var release releaseInfo

	if err := json.Unmarshal(body, &release); err != nil {
		return nil, fmt.Errorf("decode latest GOREST release response: %w", err)
	}

	if !semver.IsValid(release.TagName) {
		return nil, fmt.Errorf("latest GOREST release has invalid version tag %q", release.TagName)
	}

	if semver.Compare(installedVersion, release.TagName) >= 0 {
		return nil, nil
	}

	return &updateNotice{
		InstalledVersion: installedVersion,
		LatestVersion:    release.TagName,
		ReleaseURL:       gorestReleasePageURL + url.PathEscape(release.TagName),
	}, nil
}
