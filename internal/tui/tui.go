package tui

import (
	"database/sql"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"codeberg.org/blckr/spejder/internal/db"
)

const refreshInterval = 10 * time.Second

var (
	title  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	active = lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("4")).Foreground(lipgloss.Color("15")).Padding(0, 1)
	// selRow highlights only the text portion of a row — no padding so the bar stays aligned.
	selRow   = lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("5")).Foreground(lipgloss.Color("15"))
	inactive = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Padding(0, 1)
	header   = lipgloss.NewStyle().Bold(true).Underline(true)
	bar      = lipgloss.NewStyle().Foreground(lipgloss.Color("4"))
	dim      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	panel    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	label    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	value    = lipgloss.NewStyle().Bold(true)
	portTag  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("3"))
)

type viewMode int

const (
	viewDashboard viewMode = iota
	viewTraffic
)

type focusSide int

const (
	focusLeft focusSide = iota
	focusRight
)

type tickMsg time.Time
type dashDataMsg struct {
	countries []drillItem
	types     []drillItem
	reset     bool // true when triggered by a filter change — resets drill state
}
type trafficDataMsg struct {
	series []db.DayCount
}
type cityDrillMsg struct {
	country string
	items   []drillItem
}
type asnDrillMsg struct {
	title string
	items []drillItem
	side  focusSide
}
type ipDrillMsg struct {
	title string
	items []drillItem
	side  focusSide
}
type ipDetailMsg struct {
	summary db.IPSummary
	rdns    string
}

type model struct {
	db         *sql.DB
	timeFilter db.TimeFilter
	portFilter uint16

	// port input mode — active while user is typing a port number after pressing /
	portInputMode bool
	portInput     string

	mode  viewMode
	focus focusSide

	left  drillPanel
	right drillPanel

	series []db.DayCount

	detail    *db.IPSummary
	detailRDN string

	width  int
	height int
}

func New(database *sql.DB) model {
	return model{db: database, mode: viewDashboard, focus: focusLeft}
}

func (m model) dbFilter() db.Filter {
	return db.Filter{Time: m.timeFilter, Port: m.portFilter}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.loadDash(), m.tick())
}

func (m model) tick() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m model) loadDash() tea.Cmd {
	return m.loadDashMsg(false)
}

func (m model) loadDashReset() tea.Cmd {
	return m.loadDashMsg(true)
}

func (m model) loadDashMsg(reset bool) tea.Cmd {
	f := m.dbFilter()
	d := m.db
	return func() tea.Msg {
		countries, _ := db.TopCountries(d, f, 15)
		countryItems := make([]drillItem, len(countries))
		for i, c := range countries {
			lbl := c.Country
			if lbl == "" {
				lbl = "??"
			}
			countryItems[i] = drillItem{label: lbl, count: c.Count, key: c.Country}
		}

		traffic, _ := db.TrafficByType(d, f)
		typeItems := []drillItem{
			{label: "Internal", count: traffic.Internal, key: "internal"},
			{label: "Scanner ", count: traffic.Scanner, key: "scanner"},
			{label: "Bot     ", count: traffic.Bot, key: "bot"},
			{label: "Unknown ", count: traffic.Unknown, key: "unknown"},
		}

		return dashDataMsg{countries: countryItems, types: typeItems, reset: reset}
	}
}

func (m model) loadTraffic() tea.Cmd {
	f := m.dbFilter()
	d := m.db
	return func() tea.Msg {
		series, _ := db.TimeSeries(d, f)
		return trafficDataMsg{series: series}
	}
}

func (m model) drillCountryCmd() tea.Cmd {
	f := m.dbFilter()
	d := m.db
	depth := len(m.left.stack)
	switch depth {
	case 1: // country → cities
		country := m.left.selectedKey()
		return func() tea.Msg {
			rows, _ := db.TopCitiesForCountry(d, f, country, 10)
			items := make([]drillItem, len(rows))
			for i, r := range rows {
				lbl := r.Country
				if lbl == "" {
					lbl = "??"
				}
				items[i] = drillItem{label: lbl, count: r.Count, key: r.Country}
			}
			return cityDrillMsg{country: country, items: items}
		}
	case 2: // city → ASNs
		country := m.left.stack[1].title
		city := m.left.selectedKey()
		return func() tea.Msg {
			rows, _ := db.TopASNsForCountryCity(d, f, country, city, 10)
			items := make([]drillItem, len(rows))
			for i, r := range rows {
				lbl := r.Country
				if lbl == "" {
					lbl = "??"
				}
				items[i] = drillItem{label: lbl, count: r.Count, key: r.Country}
			}
			return asnDrillMsg{title: city, items: items, side: focusLeft}
		}
	case 3: // ASN → IPs
		asnOrg := m.left.selectedKey()
		return func() tea.Msg {
			rows, _ := db.IPsForASN(d, f, asnOrg, 20)
			items := make([]drillItem, len(rows))
			for i, r := range rows {
				items[i] = drillItem{
					label: fmt.Sprintf("%-15s :%5d  %s", r.IP, r.Port, r.SeenAt),
					count: 1,
					key:   r.IP,
				}
			}
			return ipDrillMsg{title: asnOrg, items: items, side: focusLeft}
		}
	case 4: // IP level → open detail modal
		ip := m.left.selectedKey()
		return m.loadIPDetail(ip)
	}
	return nil
}

func (m model) drillTypeCmd() tea.Cmd {
	f := m.dbFilter()
	d := m.db
	depth := len(m.right.stack)
	switch depth {
	case 1: // type → ASNs
		trafficType := m.right.selectedKey()
		return func() tea.Msg {
			rows, _ := db.TopASNsForType(d, f, trafficType, 10)
			items := make([]drillItem, len(rows))
			for i, r := range rows {
				lbl := r.Country
				if lbl == "" {
					lbl = "??"
				}
				items[i] = drillItem{label: lbl, count: r.Count, key: r.Country}
			}
			return asnDrillMsg{title: trafficType, items: items, side: focusRight}
		}
	case 2: // ASN → IPs
		asnOrg := m.right.selectedKey()
		return func() tea.Msg {
			rows, _ := db.IPsForASN(d, f, asnOrg, 20)
			items := make([]drillItem, len(rows))
			for i, r := range rows {
				items[i] = drillItem{
					label: fmt.Sprintf("%-15s :%5d  %s", r.IP, r.Port, r.SeenAt),
					count: 1,
					key:   r.IP,
				}
			}
			return ipDrillMsg{title: asnOrg, items: items, side: focusRight}
		}
	case 3: // IP level → open detail modal
		ip := m.right.selectedKey()
		return m.loadIPDetail(ip)
	}
	return nil
}

func (m model) loadIPDetail(ip string) tea.Cmd {
	d := m.db
	return func() tea.Msg {
		summary, _ := db.IPSummaryForIP(d, ip)
		rdns := ""
		if names, err := net.LookupAddr(ip); err == nil && len(names) > 0 {
			rdns = strings.TrimSuffix(names[0], ".")
		}
		return ipDetailMsg{summary: summary, rdns: rdns}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		ph := m.height - 5
		// Width(n) in lipgloss sets content+padding area; border adds 1 on each side.
		// So outer width per panel = n+2. Two panels must sum to m.width:
		// leftW+2 + rightW+2 = m.width → leftW = m.width/2-2, rightW = m.width-m.width/2-2
		leftW := m.width/2 - 2
		rightW := m.width - m.width/2 - 2
		m.left.width, m.left.height = leftW, ph
		m.right.width, m.right.height = rightW, ph

	case tickMsg:
		if m.mode == viewDashboard {
			return m, tea.Batch(m.loadDash(), m.tick())
		}
		return m, tea.Batch(m.loadTraffic(), m.tick())

	case dashDataMsg:
		if msg.reset || len(m.left.stack) == 0 {
			m.left = newDrillPanel("Countries", msg.countries, m.left.width, m.left.height)
			m.right = newDrillPanel("Traffic Type", msg.types, m.right.width, m.right.height)
		} else {
			m.left.updateRoot(msg.countries)
			m.right.updateRoot(msg.types)
		}

	case trafficDataMsg:
		m.series = msg.series

	case cityDrillMsg:
		m.left.push(msg.country, msg.items, false)

	case asnDrillMsg:
		if msg.side == focusLeft {
			m.left.push(msg.title, msg.items, false)
		} else {
			m.right.push(msg.title, msg.items, false)
		}

	case ipDrillMsg:
		if msg.side == focusLeft {
			m.left.push(msg.title, msg.items, true)
		} else {
			m.right.push(msg.title, msg.items, true)
		}

	case ipDetailMsg:
		s := msg.summary
		m.detail = &s
		m.detailRDN = msg.rdns

	case tea.KeyMsg:
		// Detail modal swallows all input.
		if m.detail != nil {
			if msg.String() == "esc" || msg.String() == "q" || msg.String() == "backspace" || msg.String() == "h" {
				m.detail = nil
			}
			return m, nil
		}

		// Port input mode — only digits, backspace, enter, esc are meaningful.
		if m.portInputMode {
			switch msg.String() {
			case "esc":
				m.portInputMode = false
				m.portInput = ""
			case "enter":
				m.portInputMode = false
				port, err := strconv.ParseUint(m.portInput, 10, 16)
				if err != nil || port > 65535 {
					port = 0
				}
				m.portFilter = uint16(port)
				m.portInput = ""
				return m, m.reloadResetCmd()
			case "backspace":
				if len(m.portInput) > 0 {
					m.portInput = m.portInput[:len(m.portInput)-1]
				}
			default:
				r := msg.String()
				if len(r) == 1 && r[0] >= '0' && r[0] <= '9' && len(m.portInput) < 5 {
					m.portInput += r
				}
			}
			return m, nil
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		// Port filter
		case "/":
			m.portInputMode = true
			m.portInput = ""
		case "0":
			m.portFilter = 0
			return m, m.reloadResetCmd()

		// Time filter: 1=all  2=month  3=week  4=3days  5=24h
		case "1":
			m.timeFilter = db.FilterAll
			return m, m.reloadResetCmd()
		case "2":
			m.timeFilter = db.FilterMonth
			return m, m.reloadResetCmd()
		case "3":
			m.timeFilter = db.FilterWeek
			return m, m.reloadResetCmd()
		case "4":
			m.timeFilter = db.Filter3Days
			return m, m.reloadResetCmd()
		case "5":
			m.timeFilter = db.Filter24h
			return m, m.reloadResetCmd()

		// Toggle traffic / dashboard view
		case "t":
			if m.mode == viewDashboard {
				m.mode = viewTraffic
				return m, m.loadTraffic()
			}
			m.mode = viewDashboard
			return m, m.loadDashReset()

		// Panel focus: i=left  o=right  tab=toggle
		case "i":
			if m.mode == viewDashboard {
				m.focus = focusLeft
			}
		case "o":
			if m.mode == viewDashboard {
				m.focus = focusRight
			}
		case "tab":
			if m.mode == viewDashboard {
				if m.focus == focusLeft {
					m.focus = focusRight
				} else {
					m.focus = focusLeft
				}
			}

		// Navigation: k=up  j=down  l=drill in  h=back
		case "k", "up":
			if m.mode == viewDashboard {
				if m.focus == focusLeft {
					m.left.moveUp()
				} else {
					m.right.moveUp()
				}
			}
		case "j", "down":
			if m.mode == viewDashboard {
				if m.focus == focusLeft {
					m.left.moveDown()
				} else {
					m.right.moveDown()
				}
			}
		case "l", "enter":
			if m.mode == viewDashboard {
				if m.focus == focusLeft {
					return m, m.drillCountryCmd()
				}
				return m, m.drillTypeCmd()
			}
		case "h", "esc", "backspace":
			if m.mode == viewDashboard {
				if m.focus == focusLeft {
					m.left.pop()
				} else {
					m.right.pop()
				}
			}
		}
	}
	return m, nil
}

func (m model) reloadResetCmd() tea.Cmd {
	if m.mode == viewTraffic {
		return m.loadTraffic()
	}
	return m.loadDashReset()
}

func (m model) View() string {
	if m.width == 0 {
		return "loading..."
	}

	base := lipgloss.JoinVertical(lipgloss.Left,
		m.viewHeader(),
		m.viewBody(),
		m.viewHelp(),
	)

	if m.detail != nil {
		return m.viewDetailOverlay(base)
	}
	return base
}

func (m model) viewHeader() string {
	filters := []db.TimeFilter{db.FilterAll, db.FilterMonth, db.FilterWeek, db.Filter3Days, db.Filter24h}
	var tabs []string
	for _, f := range filters {
		if f == m.timeFilter {
			tabs = append(tabs, active.Render(f.Label()))
		} else {
			tabs = append(tabs, inactive.Render(f.Label()))
		}
	}

	viewLabel := "dashboard"
	if m.mode == viewTraffic {
		viewLabel = "traffic"
	}
	t := title.Render("spejder") + dim.Render(" ["+viewLabel+"]")

	if m.portFilter != 0 {
		t += " " + portTag.Render(fmt.Sprintf("port:%d", m.portFilter))
	}

	filterBar := strings.Join(tabs, "")
	gap := max(m.width-lipgloss.Width(t)-lipgloss.Width(filterBar), 1)
	return t + strings.Repeat(" ", gap) + filterBar
}

func (m model) viewBody() string {
	if m.mode == viewTraffic {
		return m.viewTrafficChart()
	}
	return lipgloss.JoinHorizontal(lipgloss.Top,
		m.left.view(m.focus == focusLeft),
		m.right.view(m.focus == focusRight),
	)
}

func (m model) viewTrafficChart() string {
	chartW := m.width - 4
	content := lipgloss.JoinVertical(lipgloss.Left,
		header.Render("Connections over time"),
		"",
		lineChart(m.series, chartW),
	)
	return panel.Width(m.width - 2).Render(content)
}

func (m model) viewHelp() string {
	if m.portInputMode {
		return dim.Render("Port: ") + value.Render(m.portInput+"_") + dim.Render("  enter: apply  esc: cancel")
	}
	if m.mode == viewDashboard {
		return dim.Render("i/o/tab: panel  k/j: up/down  l: drill in  h: back  t: traffic  1-5: time  /: port  0: clear port  q: quit")
	}
	return dim.Render("t: dashboard  1-5: time  /: port  0: clear port  q: quit")
}

func detailRow(k, v string) string {
	return fmt.Sprintf("%-20s %s", label.Render(k), value.Render(v))
}

func (m model) viewDetailOverlay(base string) string {
	s := m.detail

	loc := s.Country
	if s.City != "" {
		loc = s.City + ", " + s.Country
	}

	rdns := m.detailRDN
	if rdns == "" {
		rdns = dim.Render("(no PTR record)")
	}

	lines := []string{
		header.Render("IP Details"),
		"",
		detailRow("IP Address", s.IP),
		detailRow("Location", loc),
		detailRow("Network", fmt.Sprintf("AS%d", s.ASN)),
		detailRow("ISP / Organization", s.ASNOrg),
		detailRow("Reverse DNS", rdns),
		detailRow("Connection Type", s.TrafficType),
		detailRow("Total connections", fmt.Sprintf("%d", s.Total)),
		detailRow("Last seen", s.LastSeen.Format("2006-01-02 15:04:05")),
		"",
		dim.Render("esc / q  close"),
	}

	box := panel.Width(60).Render(strings.Join(lines, "\n"))

	_ = base // replaced entirely by a centered overlay on blank background
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func Run(dbPath string) error {
	database, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	_, err = tea.NewProgram(New(database), tea.WithAltScreen()).Run()
	return err
}
