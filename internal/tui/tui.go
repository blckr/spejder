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

var (
	title    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	active   = lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("4")).Foreground(lipgloss.Color("15")).Padding(0, 1)
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

// Which panel is currently in focus
type focusPanel int

const (
	focusLeft      focusPanel = iota // Incoming - Countries
	focusRightType                   // Incoming - Types
	focusRightDur                    // Incoming - Duration
	focusRightPort                   // Incoming - Ports
)

// Model saves the state of the tui
type model struct {
	db         *sql.DB
	timeFilter db.TimeFilter // Time Filter
	portFilter uint16        // Port Filter (0 is all)

	// If user want to filter ports
	portInputMode bool
	portInput     string

	// What Panel has the focus
	focus focusPanel

	// Out Panels
	panelLeft      drillPanel
	panelRightType drillPanel
	panelRightDur  drillPanel
	panelRightPort drillPanel

	// Detail View
	detail    *db.IPSummary
	detailRDN string // Reverse DNS Name for IP

	// Window Width
	width  int
	height int
}

func New(database *sql.DB) model {
	return model{
		db:         database,
		timeFilter: db.FilterAll,
		portFilter: 0,
		focus:      focusLeft,
	}
}

type tickMsg time.Time

func (m model) Init() tea.Cmd {
	return tea.Batch(m.loadDash(), m.tick())
}

func (m model) tick() tea.Cmd {
	return tea.Tick(10*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m model) dbFilter() db.Filter {
	return db.Filter{Time: m.timeFilter, Port: m.portFilter}
}

type dashdataMsg struct {
	countries []drillItem
	types     []drillItem
	durations []drillItem
	ports     []drillItem
}

func (m model) loadDash() tea.Cmd {
	return func() tea.Msg {
		f := m.dbFilter()

		// 1. Daten aus der DB abfragen
		dbCountries, _ := db.QueryDrill(m.db, "country", f.Where(), nil, 25)
		dbTypes, _ := db.QueryDrill(m.db, "traffic_type", f.Where(), nil, 5)
		dbDurations, _ := db.ConnectionDurations(m.db)
		dbPorts, _ := db.TopPorts(m.db, 15)

		// 2. In TUI-Strukturen konvertieren
		countries := make([]drillItem, len(dbCountries))
		for i, c := range dbCountries {
			countries[i] = drillItem{label: c.Label, count: c.Count, key: c.Key}
		}

		types := make([]drillItem, len(dbTypes))
		for i, t := range dbTypes {
			types[i] = drillItem{label: t.Label, count: t.Count, key: t.Key}
		}

		durations := make([]drillItem, len(dbDurations))
		for i, d := range dbDurations {
			durations[i] = drillItem{label: d.Label, count: d.Count, key: d.Key}
		}

		ports := make([]drillItem, len(dbPorts))
		for i, p := range dbPorts {
			ports[i] = drillItem{label: p.Label, count: p.Count, key: p.Key}
		}

		return dashdataMsg{
			countries: countries,
			types:     types,
			durations: durations,
			ports:     ports,
		}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

		panelHeight := m.height - 5

		leftWidth := m.width/2 - 2
		rightWidth := m.width - m.width/2 - 2

		rightPanelHeight := panelHeight / 3

		m.panelLeft.width, m.panelLeft.height = leftWidth, panelHeight
		m.panelRightType.width, m.panelRightType.height = rightWidth, rightPanelHeight
		m.panelRightDur.width, m.panelRightDur.height = rightWidth, rightPanelHeight
		m.panelRightPort.width, m.panelRightPort.height = rightWidth, rightPanelHeight

	case tickMsg:
		if m.detail == nil {
			return m, tea.Batch(m.loadDash(), m.tick())
		}
		return m, m.tick()

	case dashdataMsg:
		if len(m.panelLeft.stack) == 0 {
			m.panelLeft = newDrillPanel("Länder", msg.countries, m.panelLeft.width, m.panelLeft.height)
			m.panelRightType = newDrillPanel("Type", msg.types, m.panelRightType.width,
				m.panelRightType.height)
			m.panelRightDur = newDrillPanel("Duration", msg.durations, m.panelRightDur.width,
				m.panelRightDur.height)
			m.panelRightPort = newDrillPanel("Ports", msg.ports, m.panelRightPort.width,
				m.panelRightPort.height)
		} else {
			m.panelLeft.updateRoot(msg.countries)
			m.panelRightType.updateRoot(msg.types)
			m.panelRightDur.updateRoot(msg.durations)
			m.panelRightPort.updateRoot(msg.ports)
		}

	// 4. Tastatureingaben verarbeiten
	case tea.KeyMsg:
		// Detail modal swallows all input.
		if m.detail != nil {
			if msg.String() == "esc" || msg.String() == "q" || msg.String() == "backspace" || msg.String() == "h" {
				m.detail = nil
			}
			return m, nil
		}

		// Port-Eingabemodus (wenn "/" gedrückt wurde)
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
				return m, m.loadDash()
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
		// Beenden
		case "q", "ctrl+c":
			return m, tea.Quit

		// Port-Filter aktivieren
		case "/":
			m.portInputMode = true
			m.portInput = ""
		case "0":
			m.portFilter = 0
			return m, m.loadDash()

		// Zeitfilter (1-8)
		case "1", "2", "3", "4", "5", "6", "7", "8":
			val, _ := strconv.Atoi(msg.String())
			m.timeFilter = db.TimeFilter(val - 1)
			return m, m.loadDash()

		// Fokus wechseln (o/tab = vorwärts, i = rückwärts)
		case "o", "tab":
			m.focus = focusPanel((int(m.focus) + 1) % 4)
		case "i":
			m.focus = focusPanel((int(m.focus) - 1 + 4) % 4)

		// Navigation im fokussierten Panel (j/k / Pfeiltasten)
		case "k", "up":
			m.focusedPanel().moveUp()
		case "j", "down":
			m.focusedPanel().moveDown()

		// Zurückgehen
		case "h", "left", "backspace":
			m.focusedPanel().pop()

		// Hineinbohren / Details öffnen
		case "l", "right", "enter":
			p := m.focusedPanel()
			cur := p.current()
			if cur != nil && len(cur.items) > 0 {
				if cur.isLeaf {
					ip := p.selectedKey()
					return m, m.loadIPDetail(ip)
				}
				return m, m.drillCmd(m.focus)
			}
		}

	// 5. Eine neue Drill-Ebene wurde geladen
	case drillMsg:
		p := m.getPanel(msg.panel)
		p.push(msg.title, msg.items, msg.leaf)
		return m, nil

	// 6. Die IP-Details für das Modal wurden geladen
	case ipDetailMsg:
		m.detail = &msg.summary
		m.detailRDN = msg.rdns
		return m, nil
	}

	return m, nil
}

// drillMsg signalisiert, dass eine neue Drill-Ebene geladen wurde.
type drillMsg struct {
	panel focusPanel
	title string
	items []drillItem
	leaf  bool
}

// ipDetailMsg signalisiert, dass IP-Details geladen wurden.
type ipDetailMsg struct {
	summary db.IPSummary
	rdns    string
}

// getPanel gibt das jeweilige Panel anhand der ID zurück
func (m *model) getPanel(id focusPanel) *drillPanel {
	switch id {
	case focusLeft:
		return &m.panelLeft
	case focusRightType:
		return &m.panelRightType
	case focusRightDur:
		return &m.panelRightDur
	case focusRightPort:
		return &m.panelRightPort
	default:
		return &m.panelLeft
	}
}

// focusedPanel gibt das aktuell aktive Panel zurück
func (m *model) focusedPanel() *drillPanel {
	return m.getPanel(m.focus)
}

// drillConditions baut die SQL-Filter dynamisch anhand des aktuellen Panel-Pfades zusammen.
func (m model) drillConditions(focus focusPanel, stack []drillLevel) (string, []any) {
	var clauses []string
	var args []any

	if m.portFilter != 0 {
		clauses = append(clauses, "dst_port = ?")
		args = append(args, m.portFilter)
	}
	timeWhere := m.timeFilter.Where()
	if timeWhere != "1=1" {
		clauses = append(clauses, timeWhere)
	}

	switch focus {
	case focusLeft:
		if len(stack) > 0 {
			clauses = append(clauses, "country = ?")
			args = append(args, stack[0].items[stack[0].sel].key)
		}
		if len(stack) > 1 {
			clauses = append(clauses, "city = ?")
			args = append(args, stack[1].items[stack[1].sel].key)
		}
		if len(stack) > 2 {
			clauses = append(clauses, "asn_org = ?")
			args = append(args, stack[2].items[stack[2].sel].key)
		}

	case focusRightType:
		clauses = append(clauses, "traffic_type = ?")
		args = append(args, stack[0].items[stack[0].sel].key)

		if len(stack) > 1 {
			clauses = append(clauses, "country = ?")
			args = append(args, stack[1].items[stack[1].sel].key)
		}
		if len(stack) > 2 {
			clauses = append(clauses, "city = ?")
			args = append(args, stack[2].items[stack[2].sel].key)
		}
		if len(stack) > 3 {
			clauses = append(clauses, "asn_org = ?")
			args = append(args, stack[3].items[stack[3].sel].key)
		}

	case focusRightDur:
		bucket := stack[0].items[stack[0].sel].key
		switch bucket {
		case "active":
			clauses = append(clauses, "closed_at IS NULL")
		case "instant":
			clauses = append(clauses, "duration_ms < 1000")
		case "short":
			clauses = append(clauses, "duration_ms >= 1000 AND duration_ms < 60000")
		case "medium":
			clauses = append(clauses, "duration_ms >= 60000 AND duration_ms < 600000")
		case "long":
			clauses = append(clauses, "duration_ms >= 600000 AND duration_ms < 3600000")
		case "persistent":
			clauses = append(clauses, "duration_ms >= 3600000")
		}

		if len(stack) > 1 {
			clauses = append(clauses, "country = ?")
			args = append(args, stack[1].items[stack[1].sel].key)
		}
		if len(stack) > 2 {
			clauses = append(clauses, "city = ?")
			args = append(args, stack[2].items[stack[2].sel].key)
		}
		if len(stack) > 3 {
			clauses = append(clauses, "asn_org = ?")
			args = append(args, stack[3].items[stack[3].sel].key)
		}

	case focusRightPort:
		port, _ := strconv.Atoi(stack[0].items[stack[0].sel].key)
		clauses = append(clauses, "dst_port = ?")
		args = append(args, port)

		if len(stack) > 1 {
			clauses = append(clauses, "country = ?")
			args = append(args, stack[1].items[stack[1].sel].key)
		}
		if len(stack) > 2 {
			clauses = append(clauses, "city = ?")
			args = append(args, stack[2].items[stack[2].sel].key)
		}
		if len(stack) > 3 {
			clauses = append(clauses, "asn_org = ?")
			args = append(args, stack[3].items[stack[3].sel].key)
		}
	}

	return strings.Join(clauses, " AND "), args
}

// drillCmd lädt die nächste Ebene im Hintergrund
func (m model) drillCmd(panelID focusPanel) tea.Cmd {
	p := m.getPanel(panelID)
	stack := p.stack
	depth := len(stack)

	return func() tea.Msg {
		where, args := m.drillConditions(panelID, stack)

		var targetCol string
		var isLeaf bool
		selectedLabel := stack[depth-1].items[stack[depth-1].sel].label

		if panelID == focusLeft {
			switch depth {
			case 1:
				targetCol = "city"
			case 2:
				targetCol = "asn_org"
			case 3:
				isLeaf = true
			}
		} else {
			switch depth {
			case 1:
				targetCol = "country"
			case 2:
				targetCol = "city"
			case 3:
				targetCol = "asn_org"
			case 4:
				isLeaf = true
			}
		}

		if isLeaf {
			dbIPs, _ := db.QueryLeaf(m.db, where, args, 20)
			items := make([]drillItem, len(dbIPs))
			for i, ip := range dbIPs {
				durationStr := ""
				if ip.Duration != "0" {
					ms, _ := strconv.ParseInt(ip.Duration, 10, 64)
					durationStr = fmt.Sprintf(" (%s)", time.Duration(ms)*time.Millisecond)
				}
				items[i] = drillItem{
					label: fmt.Sprintf("%-15s :%-5d %s%s", ip.IP, ip.Port, ip.SeenAt, durationStr),
					count: 1,
					key:   ip.IP,
				}
			}
			return drillMsg{panel: panelID, title: selectedLabel, items: items, leaf: true}
		} else {
			dbItems, _ := db.QueryDrill(m.db, targetCol, where, args, 15)
			items := make([]drillItem, len(dbItems))
			for i, item := range dbItems {
				items[i] = drillItem{label: item.Label, count: item.Count, key: item.Key}
			}
			return drillMsg{panel: panelID, title: selectedLabel, items: items, leaf: false}
		}
	}
}

// loadIPDetail lädt die IP-Details und den PTR-Eintrag (Reverse DNS) im Hintergrund
func (m model) loadIPDetail(ip string) tea.Cmd {
	return func() tea.Msg {
		summary, _ := db.IPSummaryForIP(m.db, ip)
		rdns := ""
		if names, err := net.LookupAddr(ip); err == nil && len(names) > 0 {
			rdns = strings.TrimSuffix(names[0], ".")
		}
		return ipDetailMsg{summary: summary, rdns: rdns}
	}
}

// View zeichnet das Terminal-Interface
func (m model) View() string {
	if m.width == 0 {
		return "Lade..."
	}

	// 1. Die linke Spalte rendern (Länder)
	leftView := m.panelLeft.view(m.focus == focusLeft)

	// 2. Die rechte Spalte rendern (Type + Duration + Ports untereinander)
	rightView := lipgloss.JoinVertical(lipgloss.Left,
		m.panelRightType.view(m.focus == focusRightType),
		m.panelRightDur.view(m.focus == focusRightDur),
		m.panelRightPort.view(m.focus == focusRightPort),
	)

	// 3. Spalten nebeneinander fügen
	body := lipgloss.JoinHorizontal(lipgloss.Top, leftView, rightView)

	// 4. Den Header, den Body und die Hilfezeile übereinanderstapeln
	base := lipgloss.JoinVertical(lipgloss.Left,
		m.viewHeader(),
		body,
		m.viewHelp(),
	)

	// 5. Falls das Detail-Modal einer IP offen ist, zeichnen wir es als Overlay darüber
	if m.detail != nil {
		return m.viewDetailOverlay(base)
	}

	return base
}

// viewHeader zeichnet die Kopfzeile des TUIs
func (m model) viewHeader() string {
	filters := []db.TimeFilter{
		db.FilterAll,
		db.Filter6Months,
		db.Filter3Months,
		db.Filter1Month,
		db.Filter1Week,
		db.Filter3Days,
		db.Filter1Day,
		db.Filter1Hour,
	}
	var tabs []string
	for _, f := range filters {
		if f == m.timeFilter {
			tabs = append(tabs, active.Render(f.Label()))
		} else {
			tabs = append(tabs, inactive.Render(f.Label()))
		}
	}

	t := title.Render("spejder") + dim.Render(" [incoming connections]")

	if m.portFilter != 0 {
		t += " " + portTag.Render(fmt.Sprintf("port:%d", m.portFilter))
	}

	filterBar := strings.Join(tabs, "")
	gap := max(m.width-lipgloss.Width(t)-lipgloss.Width(filterBar), 1)
	return t + strings.Repeat(" ", gap) + filterBar
}

// viewHelp zeichnet die Hilfezeile unten
func (m model) viewHelp() string {
	if m.portInputMode {
		return dim.Render("Port: ") + value.Render(m.portInput+"_") + dim.Render("  enter: filtern  esc: abbrechen")
	}
	return dim.Render("i/o/tab: Panel wechseln  hjkl/arrows: Navigieren  enter/l: Hineinbohren/Details  1-8: Zeitfilter  /: Port-Filter  0: Filter zurücksetzen  q: Beenden")
}

// detailRow zeichnet eine einzelne Zeile im IP-Detail-Modal
func detailRow(k, v string) string {
	return fmt.Sprintf("%-20s %s", label.Render(k), value.Render(v))
}

// viewDetailOverlay zeichnet das IP-Detail-Fenster zentriert über dem restlichen UI
func (m model) viewDetailOverlay(base string) string {
	s := m.detail

	loc := s.Country
	if s.City != "" {
		loc = s.City + ", " + s.Country
	}

	rdns := m.detailRDN
	if rdns == "" {
		rdns = dim.Render("(kein PTR Eintrag gefunden)")
	}

	lines := []string{
		header.Render("IP Details"),
		"",
		detailRow("IP Adresse", s.IP),
		detailRow("Standort", loc),
		detailRow("Netzwerk", fmt.Sprintf("AS%d", s.ASN)),
		detailRow("Organisation/ISP", s.ASNOrg),
		detailRow("Reverse DNS", rdns),
		detailRow("Verbindungstyp", s.TrafficType),
		detailRow("Verbindungen gesamt", fmt.Sprintf("%d", s.Total)),
		detailRow("Zuletzt gesehen", s.LastSeen.Format("2006-01-02 15:04:05")),
		"",
		dim.Render("esc / q  Schließen"),
	}

	box := panel.Width(60).Render(strings.Join(lines, "\n"))

	_ = base // Ersetzt den Hintergrund
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

// Run startet die TUI-Anwendung
func Run(dbPath string) error {
	database, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	_, err = tea.NewProgram(New(database), tea.WithAltScreen()).Run()
	return err
}
