package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	flatpakID     = "io.mrarm.mcpelauncher"
	subhomePrefix = "~/mcpe-profiles/"
)

var reName = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// --- Modelos ---

type Controller struct {
	Name string
	VID  string
	PID  string
}

func (c Controller) Label() string { return fmt.Sprintf("%s (%s/%s)", c.Name, c.VID, c.PID) }
func (c Controller) Key() string   { return c.VID + "/" + c.PID }

type Profile struct {
	Name               string
	Home               string
	IgnoredControllers []string
}

// --- Paths ---

func homeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	return h
}

func configDir() string        { return filepath.Join(homeDir(), ".config", "mcpe-profiles") }
func profilesPath() string     { return filepath.Join(configDir(), "profiles") }
func applicationsPath() string { return filepath.Join(homeDir(), ".local", "share", "applications") }

func profilePath(name string) string { return filepath.Join(profilesPath(), name) }

// profileHome é a única fonte de verdade para a home.
// Mantida como já era.
func profileHome(name string) string { return subhomePrefix + name }

func expandHome(path string) string {
	if path == "~" {
		return homeDir()
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(homeDir(), path[2:])
	}
	return path
}

func desktopPath(name string) string {
	return filepath.Join(applicationsPath(), "mcpe-profiles-"+name+".desktop")
}

// --- Validação ---

func validateProfileName(name string) error {
	if name == "" {
		return errors.New("nome do perfil vazio")
	}
	if !reName.MatchString(name) {
		return errors.New("use apenas letras, números, '-' e '_'")
	}
	return nil
}

// --- Persistência ---

func saveProfile(p Profile) error {
	if err := validateProfileName(p.Name); err != nil {
		return err
	}
	dir := profilePath(p.Name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(expandHome(p.Home), 0o755); err != nil {
		return err
	}

	toml := fmt.Sprintf("name = %q\nhome = %q\n", p.Name, p.Home)
	if err := os.WriteFile(filepath.Join(dir, "profile.toml"), []byte(toml), 0o644); err != nil {
		return err
	}

	ctrl := strings.Join(p.IgnoredControllers, "\n")
	if ctrl != "" {
		ctrl += "\n"
	}
	return os.WriteFile(filepath.Join(dir, "ignored_controllers.txt"), []byte(ctrl), 0o644)
}

func loadProfile(name string) (Profile, error) {
	if err := validateProfileName(name); err != nil {
		return Profile{}, err
	}
	dir := profilePath(name)

	p := Profile{Name: name, Home: profileHome(name)}

	data, err := os.ReadFile(filepath.Join(dir, "profile.toml"))
	if err != nil {
		// perfil ainda não salvo — só existe a pasta em ~/subhome/
		return p, nil
	}
	for _, line := range strings.Split(string(data), "\n") {
		key, val, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		val = strings.Trim(strings.TrimSpace(val), `"`)
		switch strings.TrimSpace(key) {
		case "name":
			p.Name = val
		case "home":
			// ignorado de propósito: home é sempre derivada do nome
		}
	}
	// força coerência
	p.Home = profileHome(p.Name)

	data, err = os.ReadFile(filepath.Join(dir, "ignored_controllers.txt"))
	if err != nil {
		return p, nil
	}
	for _, line := range strings.Split(string(data), "\n") {
		if v := strings.TrimSpace(line); v != "" {
			p.IgnoredControllers = append(p.IgnoredControllers, v)
		}
	}
	return p, nil
}

// listProfiles inclui perfis salvos e pastas existentes em ~/subhome/
// que ainda não têm perfil salvo.
func listProfiles() []string {
	seen := map[string]bool{}

	// 1. perfis salvos
	if entries, err := os.ReadDir(profilesPath()); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				seen[e.Name()] = true
			}
		}
	}

	// 2. pastas em ~/subhome/
	subhome := filepath.Join(homeDir(), "subhome")
	if entries, err := os.ReadDir(subhome); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				seen[e.Name()] = true
			}
		}
	}

	out := make([]string, 0, len(seen))
	for name := range seen {
		if validateProfileName(name) == nil {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func renameProfile(oldName, newName string) error {
	if err := validateProfileName(newName); err != nil {
		return err
	}
	if oldName == newName {
		return nil
	}
	if _, err := os.Stat(profilePath(newName)); err == nil {
		return fmt.Errorf("já existe um perfil com o nome %q", newName)
	}
	if err := os.Rename(profilePath(oldName), profilePath(newName)); err != nil {
		return err
	}

	oldHome := expandHome(profileHome(oldName))
	newHome := expandHome(profileHome(newName))
	if _, err := os.Stat(oldHome); err == nil {
		if err := os.MkdirAll(filepath.Dir(newHome), 0o755); err != nil {
			return err
		}
		if err := os.Rename(oldHome, newHome); err != nil {
			return fmt.Errorf("perfil renomeado, mas falha ao mover HOME: %w", err)
		}
	}

	p, err := loadProfile(newName)
	if err != nil {
		return err
	}
	p.Name = newName
	p.Home = profileHome(newName)
	return saveProfile(p)
}

// --- Desktop entry ---

func createDesktop(p Profile) error {
	if err := os.MkdirAll(applicationsPath(), 0o755); err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	content := fmt.Sprintf(
		"[Desktop Entry]\nType=Application\nName=MCPE - %s\nExec=%q -profile %s\nIcon=%s\nTerminal=false\nCategories=Game;\n",
		p.Name, exe, p.Name, flatpakID,
	)
	return os.WriteFile(desktopPath(p.Name), []byte(content), 0o755)
}

func removeDesktop(name string) { _ = os.Remove(desktopPath(name)) }

// --- Detecção de controles ---

func detectControllers() []Controller {
	entries, _ := filepath.Glob("/dev/input/event*")
	seen := map[string]bool{}
	var out []Controller

	for _, dev := range entries {
		ev := filepath.Base(dev)

		if !isGameController(ev) {
			continue
		}

		raw, err := os.ReadFile(filepath.Join("/sys/class/input", ev, "device/name"))
		if err != nil {
			continue
		}
		name := strings.TrimSpace(string(raw))
		if name == "" {
			continue
		}

		vid, pid := readUevent(ev)
		if vid == "" || pid == "" {
			continue
		}
		key := vid + "/" + pid
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, Controller{Name: name, VID: vid, PID: pid})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Label() < out[j].Label() })
	return out
}

func isGameController(ev string) bool {
	base := filepath.Join("/sys/class/input", ev, "device", "capabilities")

	evData, err := os.ReadFile(filepath.Join(base, "ev"))
	if err != nil {
		return false
	}
	keyData, _ := os.ReadFile(filepath.Join(base, "key"))
	absData, _ := os.ReadFile(filepath.Join(base, "abs"))

	evBits, _ := parseHexBitmap(string(evData))
	keyBits, _ := parseHexBitmap(string(keyData))
	absBits, _ := parseHexBitmap(string(absData))

	// Tem EV_KEY? (bit 0x01)
	if !evBits[0x01] {
		return false
	}

	// BTN_JOYSTICK (0x120..0x12b) ou BTN_GAMEPAD (0x130..0x13b)
	for b := 0x120; b <= 0x13b; b++ {
		if keyBits[b] {
			return true
		}
	}

	// ABS_HAT0X (0x10) indica D-pad → gamepad/joystick
	if absBits[0x10] {
		return true
	}

	return false
}

// parseHexBitmap converte "ff000000 0000..." em um []uint64 de bits.
func parseHexBitmap(s string) (map[int]bool, error) {
	bits := map[int]bool{}
	words := strings.Fields(strings.TrimSpace(s))
	for w, word := range words {
		val, err := strconv.ParseUint(word, 16, 64)
		if err != nil {
			return nil, err
		}
		for b := 0; b < 64; b++ {
			if val&(1<<uint(b)) != 0 {
				bits[w*64+b] = true
			}
		}
	}
	return bits, nil
}

func readUevent(eventName string) (string, string) {
	data, err := os.ReadFile(filepath.Join("/sys/class/input", eventName, "device", "uevent"))
	if err != nil {
		return "", ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		v, ok := strings.CutPrefix(strings.TrimSpace(line), "PRODUCT=")
		if !ok {
			continue
		}
		parts := strings.Split(v, "/")
		if len(parts) < 3 {
			continue
		}
		vid, err1 := strconv.ParseUint(parts[1], 16, 16)
		pid, err2 := strconv.ParseUint(parts[2], 16, 16)
		if err1 != nil || err2 != nil {
			continue
		}
		return fmt.Sprintf("%04x", vid), fmt.Sprintf("%04x", pid)
	}
	return "", ""
}

func normalizeIgnored(list []string) []string {
	var out []string
	for _, item := range list {
		vid, pid, ok := strings.Cut(strings.TrimSpace(item), "/")
		if !ok {
			continue
		}
		v, err1 := strconv.ParseUint(strings.TrimPrefix(strings.ToLower(vid), "0x"), 16, 16)
		p, err2 := strconv.ParseUint(strings.TrimPrefix(strings.ToLower(pid), "0x"), 16, 16)
		if err1 != nil || err2 != nil {
			continue
		}
		out = append(out, fmt.Sprintf("0x%04x/0x%04x", v, p))
	}
	return out
}

// --- Execução ---

func runProfile(name string) error {
	p, err := loadProfile(name)
	if err != nil {
		return err
	}
	h := expandHome(p.Home)
	if h == "" {
		return errors.New("HOME inválido")
	}

	args := []string{
		"run",
		"--env=HOME=" + h,
		"--env=SDL_JOYSTICK_ALLOW_BACKGROUND_EVENTS=1",
	}
	if ignored := normalizeIgnored(p.IgnoredControllers); len(ignored) > 0 {
		args = append(args, "--env=SDL_GAMECONTROLLER_IGNORE_DEVICES="+strings.Join(ignored, ","))
	}
	args = append(args, flatpakID)

	fmt.Println("Executando MCPE:\nflatpak", strings.Join(args, " "))

	cmd := exec.Command("flatpak", args...)
	cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
	return cmd.Run()
}
