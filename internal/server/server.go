package server

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"sophonie/sono/internal/config"
	"sophonie/sono/internal/manager"
	"sophonie/sono/internal/nodedist"
	"sophonie/sono/internal/pkgmgr"
	"sophonie/sono/web"
)

const (
	indexRefreshInterval = time.Hour
	pageSize             = 50
	pmPageSize           = 50
)

type installState struct {
	Stage      string
	Downloaded int64
	Total      int64
	Done       bool
	Err        string
	announced  bool
}

type Server struct {
	cfg       *config.Config
	templates *template.Template

	mu       sync.Mutex
	index    nodedist.Index
	schedule nodedist.Schedule
	loadedAt time.Time

	installMu sync.Mutex
	installs  map[string]*installState
}

func New(cfg *config.Config) (*Server, error) {
	templates, err := template.ParseFS(web.Templates, "*.html")
	if err != nil {
		return nil, err
	}
	return &Server{cfg: cfg, templates: templates, installs: map[string]*installState{}}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleDashboard)
	mux.HandleFunc("GET /versions", s.handleVersions)
	mux.HandleFunc("POST /install", s.handleInstall)
	mux.HandleFunc("GET /install/status", s.handleInstallStatus)
	mux.HandleFunc("POST /activate", s.handleActivate)
	mux.HandleFunc("POST /uninstall", s.handleUninstall)
	mux.HandleFunc("POST /cache/purge", s.handleCachePurge)
	mux.HandleFunc("POST /cache/settings", s.handleCacheSettings)
	mux.HandleFunc("GET /pm", s.handlePm)
	mux.HandleFunc("GET /pm/versions", s.handlePmVersions)
	mux.HandleFunc("POST /pm/install", s.handlePmInstall)
	mux.HandleFunc("POST /pm/activate", s.handlePmActivate)
	mux.HandleFunc("POST /pm/uninstall", s.handlePmUninstall)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(web.Static))))
	return mux
}

func (s *Server) ListenAndServe(addr string) error {
	go s.autoPurgeLoop()
	return http.ListenAndServe(addr, s.Handler())
}

func (s *Server) autoPurgeLoop() {
	s.runAutoPurge()
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		s.runAutoPurge()
	}
}

func (s *Server) runAutoPurge() {
	settings := config.LoadSettings(s.cfg)
	if !settings.AutoPurgeEnabled || settings.CacheMaxAgeDays <= 0 {
		return
	}
	_, _ = manager.PurgeExpired(s.cfg, settings.CacheMaxAgeDays)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	s.render(w, "dashboard.html", s.buildVersionsView(r))
}

func (s *Server) handleVersions(w http.ResponseWriter, r *http.Request) {
	s.render(w, "versions.html", s.buildVersionsView(r))
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

type versionRow struct {
	Version      string
	Date         string
	IsLTS        bool
	Codename     string
	Installed    bool
	Active       bool
	EndOfLife    string
	Expired      bool
	UpdateTarget string
}

type versionsView struct {
	Rows             []versionRow
	Filter           string
	View             string
	Query            string
	Active           string
	ResolvedPath     string
	CurrentBinOnPath bool
	PathOK           bool
	LatestLTS        string
	TotalCount       int
	MatchCount       int
	Page             int
	PageSize         int
	TotalPages       int
	HasPrev          bool
	HasNext          bool
	PrevPage         int
	NextPage         int
	CacheCount       int
	CacheSize        string
	AutoPurgeEnabled bool
	CacheMaxAgeDays  int
	LoadError        string
}

func (s *Server) buildVersionsView(r *http.Request) versionsView {
	view := versionsView{
		Filter:   sanitizeFilter(r.URL.Query().Get("filter")),
		View:     sanitizeView(r.URL.Query().Get("view")),
		Query:    strings.TrimSpace(r.URL.Query().Get("q")),
		Page:     parsePage(r.URL.Query().Get("page")),
		PageSize: pageSize,
	}

	index, schedule, err := s.currentData()
	if err != nil {
		view.LoadError = err.Error()
		return view
	}
	view.TotalCount = len(index)

	installed := s.installedSet()
	active, _ := manager.Active(s.cfg)
	view.Active = active
	view.ResolvedPath = manager.ResolvedOnPath()
	view.CurrentBinOnPath = manager.CurrentBinOnPath(s.cfg)
	view.PathOK = active != "" && view.ResolvedPath == active
	view.LatestLTS = index.LatestLTS()
	updates := manager.AvailableUpdates(s.cfg, index)

	cacheCount, cacheBytes := manager.CacheInfo(s.cfg)
	view.CacheCount = cacheCount
	view.CacheSize = humanSize(cacheBytes)
	settings := config.LoadSettings(s.cfg)
	view.AutoPurgeEnabled = settings.AutoPurgeEnabled
	view.CacheMaxAgeDays = settings.CacheMaxAgeDays

	filtered := index
	switch view.Filter {
	case "lts":
		filtered = filtered.LTS()
	case "nonlts":
		filtered = filtered.NonLTS()
	case "installed":
		filtered = onlyInstalled(index, installed)
	}
	if view.Query != "" {
		filtered = filtered.SearchPrefix(view.Query)
	} else if view.View == "compact" && view.Filter != "installed" {
		filtered = filtered.LatestPerMajor()
	}
	filtered = filtered.Sorted()

	view.MatchCount = len(filtered)
	view.TotalPages = (view.MatchCount + pageSize - 1) / pageSize
	if view.TotalPages < 1 {
		view.TotalPages = 1
	}
	if view.Page > view.TotalPages {
		view.Page = view.TotalPages
	}
	start := (view.Page - 1) * pageSize
	end := start + pageSize
	if end > view.MatchCount {
		end = view.MatchCount
	}

	view.HasPrev = view.Page > 1
	view.HasNext = view.Page < view.TotalPages
	view.PrevPage = view.Page - 1
	view.NextPage = view.Page + 1

	view.Rows = make([]versionRow, 0, end-start)
	for _, release := range filtered[start:end] {
		endOfLife := ""
		if schedule != nil {
			endOfLife = schedule.EndOfLife(release.Version)
		}
		view.Rows = append(view.Rows, versionRow{
			Version:      release.Version,
			Date:         release.Date,
			IsLTS:        release.LTS.IsLTS,
			Codename:     release.LTS.Codename,
			Installed:    installed[release.Version],
			Active:       active != "" && release.Version == active,
			EndOfLife:    endOfLife,
			Expired:      isExpired(endOfLife),
			UpdateTarget: updates[release.Version],
		})
	}
	return view
}

type installStatusView struct {
	Version    string
	Filter     string
	View       string
	Query      string
	Page       int
	Stage      string
	StageLabel string
	Downloaded int64
	Total      int64
	Percent    int
	Done       bool
	Error      string
}

func (s *Server) handleInstall(w http.ResponseWriter, r *http.Request) {
	version := r.URL.Query().Get("version")
	if !s.knownVersion(version) {
		http.Error(w, "unknown version", http.StatusBadRequest)
		return
	}
	if !s.installedSet()[version] {
		s.installMu.Lock()
		state, tracked := s.installs[version]
		if !tracked || state.Err != "" {
			s.installs[version] = &installState{Stage: manager.StageDownloading}
			go s.runInstall(version)
		}
		s.installMu.Unlock()
	}
	s.render(w, "install_status.html", s.installStatusView(r))
}

func (s *Server) handleInstallStatus(w http.ResponseWriter, r *http.Request) {
	version := r.URL.Query().Get("version")
	view := s.installStatusView(r)
	s.announceInstall(w, version)
	s.render(w, "install_status.html", view)
}

func (s *Server) announceInstall(w http.ResponseWriter, version string) {
	s.installMu.Lock()
	defer s.installMu.Unlock()
	state := s.installs[version]
	if state == nil || state.announced {
		return
	}
	if state.Err != "" {
		state.announced = true
		setToast(w, "error", version+" failed: "+state.Err)
	} else if state.Done {
		state.announced = true
		setToast(w, "success", version+" installed")
	}
}

func (s *Server) handleActivate(w http.ResponseWriter, r *http.Request) {
	version := r.URL.Query().Get("version")
	if s.installedSet()[version] {
		if err := manager.SetActive(s.cfg, version); err != nil {
			setToast(w, "error", version+" could not be activated: "+err.Error())
		} else {
			setToast(w, "success", version+" is now active")
		}
	}
	s.render(w, "versions.html", s.buildVersionsView(r))
}

func (s *Server) handleUninstall(w http.ResponseWriter, r *http.Request) {
	version := r.URL.Query().Get("version")
	if err := manager.Uninstall(s.cfg, version); err != nil {
		setToast(w, "error", err.Error())
	} else {
		setToast(w, "success", version+" removed")
	}
	s.render(w, "versions.html", s.buildVersionsView(r))
}

func (s *Server) handleCachePurge(w http.ResponseWriter, r *http.Request) {
	count, _ := manager.PurgeCache(s.cfg)
	setToast(w, "success", fmt.Sprintf("Cache cleared (%d tarball%s)", count, plural(count)))
	s.render(w, "versions.html", s.buildVersionsView(r))
}

func (s *Server) handleCacheSettings(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	_ = config.SaveSettings(s.cfg, config.Settings{
		AutoPurgeEnabled: r.PostForm.Get("autopurge") == "on",
		CacheMaxAgeDays:  clampMaxAge(r.PostForm.Get("maxage")),
	})
	setToast(w, "success", "Settings saved")
	s.render(w, "versions.html", s.buildVersionsView(r))
}

func setToast(w http.ResponseWriter, level, message string) {
	payload, err := json.Marshal(map[string]any{
		"toast": map[string]string{"level": level, "message": message},
	})
	if err != nil {
		return
	}
	w.Header().Set("HX-Trigger", string(payload))
}

type pmVersionRow struct {
	Version   string
	Installed bool
	Active    bool
}

type pmView struct {
	PM          string
	Active      string
	ShimsOnPath bool
	Query       string
	Rows        []pmVersionRow
	MatchCount  int
	Page        int
	PageSize    int
	TotalPages  int
	HasPrev     bool
	HasNext     bool
	PrevPage    int
	NextPage    int
	LoadError   string
}

func (s *Server) buildPmView(r *http.Request) pmView {
	pm, ok := pkgmgr.Find(r.URL.Query().Get("pm"))
	if !ok {
		pm = pkgmgr.Supported[0]
	}
	view := pmView{
		PM:       pm.Name,
		Query:    strings.TrimSpace(r.URL.Query().Get("q")),
		Page:     parsePage(r.URL.Query().Get("page")),
		PageSize: pmPageSize,
	}

	versions, err := pkgmgr.ListStableVersions(s.cfg, pm)
	if err != nil {
		view.LoadError = err.Error()
		return view
	}

	active, _ := pkgmgr.Active(s.cfg, pm)
	view.Active = active
	view.ShimsOnPath = manager.DirOnPath(s.cfg.ShimsDir)
	installed := pmInstalledSet(s.cfg, pm)

	filtered := versions
	if view.Query != "" {
		matched := make([]string, 0, len(versions))
		for _, version := range versions {
			if strings.HasPrefix(version, view.Query) {
				matched = append(matched, version)
			}
		}
		filtered = matched
	}

	view.MatchCount = len(filtered)
	view.TotalPages = (view.MatchCount + pmPageSize - 1) / pmPageSize
	if view.TotalPages < 1 {
		view.TotalPages = 1
	}
	if view.Page > view.TotalPages {
		view.Page = view.TotalPages
	}
	start := (view.Page - 1) * pmPageSize
	end := start + pmPageSize
	if end > view.MatchCount {
		end = view.MatchCount
	}

	view.HasPrev = view.Page > 1
	view.HasNext = view.Page < view.TotalPages
	view.PrevPage = view.Page - 1
	view.NextPage = view.Page + 1

	view.Rows = make([]pmVersionRow, 0, end-start)
	for _, version := range filtered[start:end] {
		view.Rows = append(view.Rows, pmVersionRow{
			Version:   version,
			Installed: installed[version],
			Active:    active != "" && version == active,
		})
	}
	return view
}

func pmInstalledSet(cfg *config.Config, pm pkgmgr.PackageManager) map[string]bool {
	set := map[string]bool{}
	installed, err := pkgmgr.Installed(cfg, pm)
	if err != nil {
		return set
	}
	for _, version := range installed {
		set[version] = true
	}
	return set
}

func (s *Server) handlePm(w http.ResponseWriter, r *http.Request) {
	s.render(w, "pm.html", s.buildPmView(r))
}

func (s *Server) handlePmVersions(w http.ResponseWriter, r *http.Request) {
	s.render(w, "pm_versions.html", s.buildPmView(r))
}

func (s *Server) handlePmInstall(w http.ResponseWriter, r *http.Request) {
	pm, ok := pkgmgr.Find(r.URL.Query().Get("pm"))
	version := r.URL.Query().Get("version")
	if ok {
		if err := pkgmgr.Install(s.cfg, pm, version); err != nil {
			setToast(w, "error", pm.Name+" "+version+" failed: "+err.Error())
		} else {
			setToast(w, "success", pm.Name+" "+version+" installed")
		}
	}
	s.render(w, "pm_versions.html", s.buildPmView(r))
}

func (s *Server) handlePmActivate(w http.ResponseWriter, r *http.Request) {
	pm, ok := pkgmgr.Find(r.URL.Query().Get("pm"))
	version := r.URL.Query().Get("version")
	if ok {
		if err := pkgmgr.Activate(s.cfg, pm, version); err != nil {
			setToast(w, "error", pm.Name+" could not be activated: "+err.Error())
		} else {
			setToast(w, "success", pm.Name+" "+version+" is now active")
		}
	}
	s.render(w, "pm_versions.html", s.buildPmView(r))
}

func (s *Server) handlePmUninstall(w http.ResponseWriter, r *http.Request) {
	pm, ok := pkgmgr.Find(r.URL.Query().Get("pm"))
	version := r.URL.Query().Get("version")
	if ok {
		if err := pkgmgr.Uninstall(s.cfg, pm, version); err != nil {
			setToast(w, "error", err.Error())
		} else {
			setToast(w, "success", pm.Name+" "+version+" removed")
		}
	}
	s.render(w, "pm_versions.html", s.buildPmView(r))
}

func (s *Server) runInstall(version string) {
	err := manager.Install(s.cfg, version, func(stage string, downloaded, total int64) {
		s.installMu.Lock()
		if state := s.installs[version]; state != nil {
			state.Stage = stage
			state.Downloaded = downloaded
			state.Total = total
		}
		s.installMu.Unlock()
	})

	s.installMu.Lock()
	if state := s.installs[version]; state != nil {
		if err != nil {
			state.Err = err.Error()
		} else {
			state.Done = true
		}
	}
	s.installMu.Unlock()
}

func (s *Server) installStatusView(r *http.Request) installStatusView {
	version := r.URL.Query().Get("version")
	view := installStatusView{
		Version: version,
		Filter:  sanitizeFilter(r.URL.Query().Get("filter")),
		View:    sanitizeView(r.URL.Query().Get("view")),
		Query:   strings.TrimSpace(r.URL.Query().Get("q")),
		Page:    parsePage(r.URL.Query().Get("page")),
	}

	s.installMu.Lock()
	state := s.installs[version]
	s.installMu.Unlock()

	if state != nil && state.Err != "" {
		view.Error = state.Err
		return view
	}
	if s.installedSet()[version] {
		view.Done = true
		return view
	}
	if state == nil || state.Done {
		view.Done = state != nil && state.Done
		return view
	}

	view.Stage = state.Stage
	view.StageLabel = stageLabel(state.Stage)
	view.Downloaded = state.Downloaded
	view.Total = state.Total
	if state.Total > 0 {
		view.Percent = int(state.Downloaded * 100 / state.Total)
	}
	return view
}

func (s *Server) knownVersion(version string) bool {
	index, _, err := s.currentData()
	if err != nil {
		return false
	}
	for _, release := range index {
		if release.Version == version {
			return true
		}
	}
	return false
}

func (s *Server) currentData() (nodedist.Index, nodedist.Schedule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.index != nil && time.Since(s.loadedAt) < indexRefreshInterval {
		return s.index, s.schedule, nil
	}

	index, err := nodedist.LoadIndex(s.cfg.IndexCache)
	if err != nil {
		if s.index != nil {
			return s.index, s.schedule, nil
		}
		return nil, nil, err
	}

	schedule, err := nodedist.LoadSchedule(s.cfg.ScheduleCache)
	if err != nil {
		schedule = nil
	}

	s.index = index
	s.schedule = schedule
	s.loadedAt = time.Now()
	return index, schedule, nil
}

func (s *Server) installedSet() map[string]bool {
	set := map[string]bool{}
	installed, err := manager.ListInstalled(s.cfg)
	if err != nil {
		return set
	}
	for _, version := range installed {
		set[version] = true
	}
	return set
}

func isExpired(endDate string) bool {
	if endDate == "" {
		return false
	}
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return false
	}
	return end.Before(time.Now())
}

func parsePage(value string) int {
	page, err := strconv.Atoi(value)
	if err != nil || page < 1 {
		return 1
	}
	return page
}

func clampMaxAge(value string) int {
	days, err := strconv.Atoi(value)
	if err != nil || days < 1 {
		return 30
	}
	if days > 3650 {
		return 3650
	}
	return days
}

func humanSize(bytes int64) string {
	if bytes <= 0 {
		return "0 MB"
	}
	return fmt.Sprintf("%.1f MB", float64(bytes)/(1<<20))
}

func stageLabel(stage string) string {
	switch stage {
	case manager.StageDownloading:
		return "Downloading"
	case manager.StageVerifying:
		return "Verifying"
	case manager.StageExtracting:
		return "Extracting"
	default:
		return "Preparing"
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func onlyInstalled(index nodedist.Index, installed map[string]bool) nodedist.Index {
	var out nodedist.Index
	for _, release := range index {
		if installed[release.Version] {
			out = append(out, release)
		}
	}
	return out
}

func sanitizeFilter(value string) string {
	switch value {
	case "lts", "nonlts", "installed", "all":
		return value
	default:
		return "all"
	}
}

func sanitizeView(value string) string {
	switch value {
	case "compact", "all":
		return value
	default:
		return "compact"
	}
}
