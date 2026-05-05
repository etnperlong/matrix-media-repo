package test

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"github.com/t2bot/matrix-media-repo/common/rcontext"
	"github.com/t2bot/matrix-media-repo/database"
	"github.com/t2bot/matrix-media-repo/test/test_internals"
	"github.com/t2bot/matrix-media-repo/util"
)

type MSC3916MiscClientEndpointsSuite struct {
	suite.Suite
	deps     *test_internals.ContainerDeps
	htmlPage *test_internals.HostedFile
}

func (s *MSC3916MiscClientEndpointsSuite) SetupSuite() {
	deps, err := test_internals.MakeTestDeps()
	if err != nil {
		log.Fatal(err)
	}
	s.deps = deps

	file, err := test_internals.ServeFile("index.html", deps, "<h1>This is a test file</h1>")
	if err != nil {
		log.Fatal(err)
	}
	s.htmlPage = file
}

func (s *MSC3916MiscClientEndpointsSuite) TearDownSuite() {
	if s.htmlPage != nil {
		if s.T().Failed() {
			staticLogs, err := s.htmlPage.Logs()
			s.deps.DumpDebugLogs(staticLogs, err, -1, s.htmlPage.PublicUrl)
		}
		s.htmlPage.Teardown()
	}
	if s.deps != nil {
		if s.T().Failed() {
			s.deps.Debug()
		}
		s.deps.Teardown()
	}
}

func (s *MSC3916MiscClientEndpointsSuite) TestPreviewUrlRequiresAuth() {
	t := s.T()

	client1 := s.deps.Homeservers[0].UnprivilegedUsers[0].WithCsUrl(s.deps.Machines[0].HttpUrl)
	client2 := &test_internals.MatrixClient{
		ClientServerUrl: s.deps.Machines[0].HttpUrl,
		ServerName:      s.deps.Homeservers[0].ServerName,
		AccessToken:     "", // no auth on this client
		UserId:          "", // no auth on this client
	}
	clientGuest := s.deps.Homeservers[0].GuestUsers[0].WithCsUrl(s.deps.Machines[0].HttpUrl)

	qs := url.Values{
		"url": []string{s.htmlPage.PublicUrl},
	}
	raw, err := client2.DoRaw("GET", "/_matrix/client/v1/media/preview_url", qs, "", nil)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, raw.StatusCode)

	raw, err = clientGuest.DoRaw("GET", "/_matrix/client/v1/media/preview_url", qs, "", nil)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, raw.StatusCode)

	raw, err = client1.DoRaw("GET", "/_matrix/client/v1/media/preview_url", qs, "", nil)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, raw.StatusCode)
}

func (s *MSC3916MiscClientEndpointsSuite) TestConfigRequiresAuth() {
	t := s.T()

	client1 := s.deps.Homeservers[0].UnprivilegedUsers[0].WithCsUrl(s.deps.Machines[0].HttpUrl)
	client2 := &test_internals.MatrixClient{
		ClientServerUrl: s.deps.Machines[0].HttpUrl,
		ServerName:      s.deps.Homeservers[0].ServerName,
		AccessToken:     "", // no auth on this client
		UserId:          "", // no auth on this client
	}
	clientGuest := s.deps.Homeservers[0].GuestUsers[0].WithCsUrl(s.deps.Machines[0].HttpUrl)

	raw, err := client2.DoRaw("GET", "/_matrix/client/v1/media/config", nil, "", nil)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, raw.StatusCode)

	raw, err = clientGuest.DoRaw("GET", "/_matrix/client/v1/media/config", nil, "", nil)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, raw.StatusCode)

	raw, err = client1.DoRaw("GET", "/_matrix/client/v1/media/config", nil, "", nil)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, raw.StatusCode)
}

func (s *MSC3916MiscClientEndpointsSuite) TestPreviewUrlConcurrentRequestsDeduplicateSameLanguage() {
	t := s.T()

	fixture := s.makePreviewFixture("concurrent-same-language")
	defer fixture.Teardown()

	client1 := s.deps.Homeservers[0].UnprivilegedUsers[0].WithCsUrl(s.deps.Machines[0].HttpUrl)
	client2 := s.deps.Homeservers[0].UnprivilegedUsers[0].WithCsUrl(s.deps.Machines[1].HttpUrl)
	ts := util.NowMillis()
	baseline, err := database.GetInstance().Media.Prepare(rcontext.Initial()).GetByUserId(client1.UserId)
	assert.NoError(t, err)

	const concurrentRequests = 12
	results := make([]MatrixOpenGraph, concurrentRequests)
	errs := make([]error, concurrentRequests)
	clients := []*test_internals.MatrixClient{client1, client2}
	start := new(sync.WaitGroup)
	start.Add(1)
	waiter := new(sync.WaitGroup)
	for i := 0; i < concurrentRequests; i++ {
		waiter.Add(1)
		go func(idx int) {
			defer waiter.Done()
			start.Wait()
			results[idx], errs[idx] = doPreviewRequest(clients[idx%len(clients)], fixture.Page.PublicUrl, ts, "en")
		}(i)
	}
	start.Done()
	waiter.Wait()

	for i := 0; i < concurrentRequests; i++ {
		assert.NoError(t, errs[i])
		assert.Equal(t, fixture.Page.PublicUrl, results[i].Url)
		assert.Equal(t, "Preview concurrent-same-language", results[i].Title)
		assert.NotEmpty(t, results[i].ImageMxc)
	}
	for i := 1; i < concurrentRequests; i++ {
		assert.Equal(t, results[0].ImageMxc, results[i].ImageMxc)
	}

	mediaRecords, err := database.GetInstance().Media.Prepare(rcontext.Initial()).GetByUserId(client1.UserId)
	assert.NoError(t, err)
	assert.Len(t, mediaRecords, len(baseline)+1)

	previewRecord, err := database.GetInstance().UrlPreviews.Prepare(rcontext.Initial()).Get(fixture.Page.PublicUrl, util.GetHourBucket(ts), "en")
	assert.NoError(t, err)
	assert.NotNil(t, previewRecord)
	assert.Equal(t, "en", previewRecord.LanguageHeader)
	assert.Equal(t, results[0].ImageMxc, previewRecord.ImageMxc)
	assert.Empty(t, previewRecord.ErrorCode)
	assert.Equal(t, fixture.Page.PublicUrl, previewRecord.SiteUrl)
	assert.Equal(t, "Preview concurrent-same-language", previewRecord.Title)
	previewRecordFr, err := database.GetInstance().UrlPreviews.Prepare(rcontext.Initial()).Get(fixture.Page.PublicUrl, util.GetHourBucket(ts), "fr")
	assert.NoError(t, err)
	assert.Nil(t, previewRecordFr)
}

func (s *MSC3916MiscClientEndpointsSuite) TestPreviewUrlCachesDifferentLanguagesSeparately() {
	t := s.T()

	fixture := s.makePreviewFixture("different-languages")
	defer fixture.Teardown()

	client1 := s.deps.Homeservers[0].UnprivilegedUsers[0].WithCsUrl(s.deps.Machines[0].HttpUrl)
	client2 := s.deps.Homeservers[0].UnprivilegedUsers[0].WithCsUrl(s.deps.Machines[1].HttpUrl)
	ts := util.NowMillis()

	resultEn, err := doPreviewRequest(client1, fixture.Page.PublicUrl, ts, "en")
	assert.NoError(t, err)
	resultFr, err := doPreviewRequest(client2, fixture.Page.PublicUrl, ts, "fr")
	assert.NoError(t, err)
	assert.NotEmpty(t, resultEn.ImageMxc)
	assert.NotEmpty(t, resultFr.ImageMxc)

	previewDb := database.GetInstance().UrlPreviews.Prepare(rcontext.Initial())
	recordEn, err := previewDb.Get(fixture.Page.PublicUrl, util.GetHourBucket(ts), "en")
	assert.NoError(t, err)
	assert.NotNil(t, recordEn)
	recordFr, err := previewDb.Get(fixture.Page.PublicUrl, util.GetHourBucket(ts), "fr")
	assert.NoError(t, err)
	assert.NotNil(t, recordFr)
	assert.Equal(t, "en", recordEn.LanguageHeader)
	assert.Equal(t, "fr", recordFr.LanguageHeader)
	assert.Equal(t, resultEn.ImageMxc, recordEn.ImageMxc)
	assert.Equal(t, resultFr.ImageMxc, recordFr.ImageMxc)
	assert.Equal(t, fixture.Page.PublicUrl, recordEn.SiteUrl)
	assert.Equal(t, fixture.Page.PublicUrl, recordFr.SiteUrl)
	assert.Empty(t, recordEn.ErrorCode)
	assert.Empty(t, recordFr.ErrorCode)
}

type previewFixture struct {
	Page  *test_internals.HostedFile
	Image *test_internals.HostedFile
}

func (f *previewFixture) Teardown() {
	if f.Page != nil {
		f.Page.Teardown()
	}
	if f.Image != nil {
		f.Image.Teardown()
	}
}

func (s *MSC3916MiscClientEndpointsSuite) makePreviewFixture(name string) *previewFixture {
	t := s.T()

	contentType, img, err := test_internals.MakeTestImage(128, 128)
	assert.NoError(t, err)
	assert.Equal(t, "image/png", contentType)
	b, err := io.ReadAll(img)
	assert.NoError(t, err)

	imageFile, err := test_internals.ServeFile(name+".png", s.deps, string(b))
	assert.NoError(t, err)

	html := strings.Join([]string{
		"<!doctype html>",
		"<html><head>",
		"<meta property=\"og:url\" content=\"{{URL}}\">",
		"<meta property=\"og:title\" content=\"Preview {{NAME}}\">",
		"<meta property=\"og:description\" content=\"Preview description {{NAME}}\">",
		"<meta property=\"og:image\" content=\"{{IMAGE}}\">",
		"<meta property=\"og:type\" content=\"website\">",
		"</head><body><h1>Preview {{NAME}}</h1></body></html>",
	}, "")
	html = strings.ReplaceAll(html, "{{NAME}}", name)
	html = strings.ReplaceAll(html, "{{IMAGE}}", imageFile.PublicUrl)

	pageFile, writeFn, err := test_internals.LazyServeFile(name+".html", s.deps)
	assert.NoError(t, err)
	html = strings.ReplaceAll(html, "{{URL}}", pageFile.PublicUrl)
	assert.NoError(t, writeFn(html))

	return &previewFixture{Page: pageFile, Image: imageFile}
}

type MatrixOpenGraph struct {
	Url         string `json:"og:url,omitempty"`
	SiteName    string `json:"og:site_name,omitempty"`
	Type        string `json:"og:type,omitempty"`
	Description string `json:"og:description,omitempty"`
	Title       string `json:"og:title,omitempty"`
	ImageMxc    string `json:"og:image,omitempty"`
	ImageType   string `json:"og:image:type,omitempty"`
	ImageSize   int64  `json:"matrix:image:size,omitempty"`
	ImageWidth  int    `json:"og:image:width,omitempty"`
	ImageHeight int    `json:"og:image:height,omitempty"`
}

func doPreviewRequest(client *test_internals.MatrixClient, previewURL string, ts int64, languageHeader string) (MatrixOpenGraph, error) {
	qs := url.Values{
		"url": []string{previewURL},
		"ts":  []string{strconv.FormatInt(ts, 10)},
	}
	endpoint, err := url.JoinPath(client.ClientServerUrl, "/_matrix/client/v1/media/preview_url")
	if err != nil {
		return MatrixOpenGraph{}, err
	}
	req, err := http.NewRequest(http.MethodGet, endpoint+"?"+qs.Encode(), nil)
	if err != nil {
		return MatrixOpenGraph{}, err
	}
	req.Host = client.ServerName
	req.Header.Set("Authorization", "Bearer "+client.AccessToken)
	req.Header.Set("Accept-Language", languageHeader)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return MatrixOpenGraph{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(res.Body)
		if readErr != nil {
			return MatrixOpenGraph{}, readErr
		}
		return MatrixOpenGraph{}, fmt.Errorf("%d : %s", res.StatusCode, string(body))
	}

	var result MatrixOpenGraph
	err = json.NewDecoder(res.Body).Decode(&result)
	return result, err
}

func TestMSC3916MiscClientEndpointsSuite(t *testing.T) {
	suite.Run(t, new(MSC3916MiscClientEndpointsSuite))
}
