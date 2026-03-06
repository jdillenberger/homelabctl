package wizard

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"

	"github.com/jdillenberger/homelabctl/internal/app"
	"github.com/jdillenberger/homelabctl/internal/config"
)

// RunSetupWizard runs the full first-time setup flow and populates cfg.
func RunSetupWizard(cfg *config.Config, availableApps []string) (selectedApps []string, err error) {
	defaults := config.DefaultConfig()
	hostname := defaults.Hostname
	appsDir := defaults.AppsDir
	dataDir := defaults.DataDir
	domain := defaults.Network.Domain
	webPortStr := strconv.Itoa(defaults.Network.WebPort)
	backupEnabled := defaults.Backup.Enabled
	borgRepo := defaults.Backup.BorgRepo
	schedule := defaults.Backup.Schedule
	routingEnabled := defaults.Routing.Enabled
	routingDomain := ""
	httpsEnabled := false
	acmeEmail := ""
	var confirmed bool

	// Step 1: Welcome
	welcomeForm := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("homelabctl Setup Wizard").
				Description("This wizard will guide you through the initial configuration.\nYou can re-run this at any time with 'homelabctl setup'."),
		),
	)
	if err := welcomeForm.Run(); err != nil {
		return nil, fmt.Errorf("setup wizard cancelled: %w", err)
	}

	// Step 2: Basic config
	basicForm := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Hostname").
				Description("The hostname for this machine.").
				Value(&hostname),
			huh.NewInput().
				Title("Apps directory").
				Description("Where deployed app configurations are stored.").
				Value(&appsDir),
			huh.NewInput().
				Title("Data directory").
				Description("Where app data volumes are stored.").
				Value(&dataDir),
			huh.NewInput().
				Title("Network domain").
				Description("Local network domain for mDNS.").
				Value(&domain),
			huh.NewInput().
				Title("Web UI port").
				Description("Port for the homelabctl web interface.").
				Value(&webPortStr),
		),
	)
	if err := basicForm.Run(); err != nil {
		return nil, fmt.Errorf("setup wizard cancelled: %w", err)
	}

	webPort, err := strconv.Atoi(webPortStr)
	if err != nil || webPort < 1 || webPort > 65535 {
		webPort = defaults.Network.WebPort
	}

	// Step 3: App selection
	appOptions := make([]huh.Option[string], len(availableApps))
	for i, name := range availableApps {
		appOptions[i] = huh.NewOption(name, name)
	}

	if len(appOptions) > 0 {
		appForm := huh.NewForm(
			huh.NewGroup(
				huh.NewMultiSelect[string]().
					Title("Select apps to deploy").
					Description("Choose which apps to deploy during setup.").
					Options(appOptions...).
					Value(&selectedApps),
			),
		)
		if err := appForm.Run(); err != nil {
			return nil, fmt.Errorf("setup wizard cancelled: %w", err)
		}
	}

	// Step 4: Backup config
	backupForm := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Enable backups?").
				Description("Configure automated Borg backups.").
				Value(&backupEnabled),
		),
	)
	if err := backupForm.Run(); err != nil {
		return nil, fmt.Errorf("setup wizard cancelled: %w", err)
	}

	if backupEnabled {
		borgForm := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Borg repository path").
					Description("Path to the Borg backup repository.").
					Value(&borgRepo),
				huh.NewInput().
					Title("Backup schedule (cron)").
					Description("Cron expression for backup schedule.").
					Value(&schedule),
			),
		)
		if err := borgForm.Run(); err != nil {
			return nil, fmt.Errorf("setup wizard cancelled: %w", err)
		}
	}

	// Step 5: Routing config
	routingForm := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Enable reverse proxy?").
				Description("Automatically expose apps at subdomain.domain via Traefik.").
				Value(&routingEnabled),
		),
	)
	if err := routingForm.Run(); err != nil {
		return nil, fmt.Errorf("setup wizard cancelled: %w", err)
	}

	if routingEnabled {
		defaultDomain := hostname + "." + domain
		routingDomain = defaultDomain
		routingDetailForm := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Base domain for apps").
					Description("Apps will be available at <app>.<domain>.").
					Value(&routingDomain),
				huh.NewConfirm().
					Title("Enable HTTPS?").
					Description("Automatic local CA certificates for .local domains.\nOptionally add Let's Encrypt for external domains.").
					Value(&httpsEnabled),
			),
		)
		if err := routingDetailForm.Run(); err != nil {
			return nil, fmt.Errorf("setup wizard cancelled: %w", err)
		}

		if httpsEnabled {
			acmeForm := huh.NewForm(
				huh.NewGroup(
					huh.NewInput().
						Title("ACME email (optional)").
						Description("For Let's Encrypt certificates on external (non-.local) domains.\nLeave empty to use local CA for all domains.").
						Value(&acmeEmail),
				),
			)
			if err := acmeForm.Run(); err != nil {
				return nil, fmt.Errorf("setup wizard cancelled: %w", err)
			}
		}
	}

	// Step 6: Summary and confirmation
	summary := fmt.Sprintf(
		"Hostname:    %s\nApps dir:    %s\nData dir:    %s\nDomain:      %s\nWeb port:    %d\nBackup:      %v\nRouting:     %v\n",
		hostname, appsDir, dataDir, domain, webPort, backupEnabled, routingEnabled,
	)
	if backupEnabled {
		summary += fmt.Sprintf("Borg repo:   %s\nSchedule:    %s\n", borgRepo, schedule)
	}
	if routingEnabled {
		httpsStr := "no"
		if httpsEnabled {
			httpsStr = "yes"
			if acmeEmail != "" {
				httpsStr += " (ACME: " + acmeEmail + ")"
			}
		}
		summary += fmt.Sprintf("Routing domain: %s\nHTTPS:          %s\n", routingDomain, httpsStr)
	}
	if len(selectedApps) > 0 {
		summary += fmt.Sprintf("Apps:        %s\n", strings.Join(selectedApps, ", "))
	}

	confirmForm := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Configuration Summary").
				Description(summary),
			huh.NewConfirm().
				Title("Write this configuration?").
				Value(&confirmed),
		),
	)
	if err := confirmForm.Run(); err != nil {
		return nil, fmt.Errorf("setup wizard cancelled: %w", err)
	}

	if !confirmed {
		return nil, fmt.Errorf("setup cancelled by user")
	}

	// Apply to config
	cfg.Hostname = hostname
	cfg.AppsDir = appsDir
	cfg.DataDir = dataDir
	cfg.Network.Domain = domain
	cfg.Network.WebPort = webPort
	cfg.Backup.Enabled = backupEnabled
	if backupEnabled {
		cfg.Backup.BorgRepo = borgRepo
		cfg.Backup.Schedule = schedule
	}
	cfg.Routing.Enabled = routingEnabled
	if routingEnabled {
		cfg.Routing.Provider = "traefik"
		cfg.Routing.Domain = routingDomain
		cfg.Routing.HTTPS.Enabled = httpsEnabled
		cfg.Routing.HTTPS.AcmeEmail = acmeEmail
	}

	return selectedApps, nil
}

// RunDeployWizard runs an interactive per-app deploy flow, prompting for each
// value defined in the app metadata and returning the collected values.
func RunDeployWizard(meta *app.AppMeta) (map[string]string, error) {
	if len(meta.Values) == 0 {
		return map[string]string{}, nil
	}

	// Allocate a string for each value. huh writes through the pointer,
	// so we keep a parallel slice and copy into the result map after Run().
	type entry struct {
		name string
		val  string
	}
	entries := make([]entry, len(meta.Values))
	for i, v := range meta.Values {
		entries[i] = entry{name: v.Name, val: v.Default}
	}

	// Build form fields, each pointing to entries[i].val.
	var fields []huh.Field
	for i, v := range meta.Values {
		input := huh.NewInput().
			Title(v.Name).
			Description(v.Description).
			Value(&entries[i].val)

		if v.Secret {
			input = input.EchoMode(huh.EchoModePassword)
		}

		fields = append(fields, input)
	}

	form := huh.NewForm(
		huh.NewGroup(fields...),
	)
	if err := form.Run(); err != nil {
		return nil, fmt.Errorf("deploy wizard cancelled: %w", err)
	}

	// Collect results from the entries slice into the map.
	values := make(map[string]string, len(entries))
	for _, e := range entries {
		values[e.name] = e.val
	}

	// Show summary
	fmt.Fprintf(os.Stderr, "\nDeploy values for %s:\n", meta.Name)
	for _, v := range meta.Values {
		display := values[v.Name]
		if v.Secret && display != "" {
			display = "********"
		}
		fmt.Fprintf(os.Stderr, "  %-20s %s\n", v.Name+":", display)
	}
	fmt.Fprintln(os.Stderr)

	return values, nil
}
