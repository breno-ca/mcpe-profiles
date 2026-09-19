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
	"syscall"
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

// subhomeRoot é a raiz onde ficam as HOMEs isoladas dos perfis.
func subhomeRoot() string { return filepath.Join(homeDir(), "mcpe-profiles") }

// profileHome devolve o caminho da HOME isolada de um perfil.
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
	if err := ensureProfileHome(p.Name); err != nil {
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
		// perfil ainda não salvo — só existe a pasta em ~/mcpe-profiles/
		if os.IsNotExist(err) {
			return p, nil
		}
		return Profile{}, err
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
		if os.IsNotExist(err) {
			return p, nil
		}
		return Profile{}, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if v := strings.TrimSpace(line); v != "" {
			p.IgnoredControllers = append(p.IgnoredControllers, v)
		}
	}
	return p, nil
}

// ensureProfileHome cria a árvore de diretórios que o mcpelauncher
// espera encontrar quando XDG_* apontam para dentro da HOME isolada.
func ensureProfileHome(name string) error {
	root := expandHome(profileHome(name))
	dirs := []string{
		root,
		filepath.Join(root, ".config"),
		filepath.Join(root, ".local", "share"),
		filepath.Join(root, ".cache"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// listProfiles inclui perfis salvos em ~/.config/mcpe-profiles/profiles/
// e pastas já existentes em ~/mcpe-profiles/.
func listProfiles() []string {
	seen := map[string]bool{}

	if entries, err := os.ReadDir(profilesPath()); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				seen[e.Name()] = true
			}
		}
	}

	if entries, err := os.ReadDir(subhomeRoot()); err == nil {
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

	// carrega ANTES de mover, para preservar controllers
	old, err := loadProfile(oldName)
	if err != nil {
		return fmt.Errorf("carregar perfil antigo: %w", err)
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

	// recria o perfil com o novo nome, PRESERVANDO os controllers
	old.Name = newName
	old.Home = profileHome(newName)
	return saveProfile(old)
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
		"[Desktop Entry]\n"+
			"Type=Application\n"+
			"Name=MCPE - %s\n"+
			"Exec=%q -profile %s\n"+
			"Icon=%s\n"+
			"Terminal=false\n"+
			"Categories=Game;\n",
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

	if !evBits[0x01] {
		return false
	}

	for b := 0x120; b <= 0x13b; b++ {
		if keyBits[b] {
			return true
		}
	}

	return absBits[0x10]
}

func parseHexBitmap(s string) (map[int]bool, error) {
	bits := map[int]bool{}
	words := strings.Fields(strings.TrimSpace(s))
	for w, word := range words {
		val, err := strconv.ParseUint(word, 16, 64)
		if err != nil {
			return nil, err
		}
		for b := range 64 {
			if val&(1<<b) != 0 {
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

// runprofile prepara a home isolada do perfil e executa o mcpelauncher
// via flatpak, passando home e xdg_* no ambiente do processo (não como
// --env do flatpak, que sobrescreveria tudo).
func runProfile(name string) error {
	p, err := loadProfile(name)
	if err != nil {
		return err
	}

	// 1. garante a árvore da HOME (cria se não existir)
	if err := ensureProfileHome(name); err != nil {
		return fmt.Errorf("preparar HOME: %w", err)
	}

	// 2. garante o perfil salvo — SÓ se ainda não existir.
	//    Não sobrescreve config do usuário.
	if _, err := os.Stat(filepath.Join(profilePath(name), "profile.toml")); err != nil {
		if err := saveProfile(p); err != nil {
			return fmt.Errorf("salvar perfil: %w", err)
		}
	}

	// 3. garante o .desktop — SÓ se ainda não existir.
	if _, err := os.Stat(desktopPath(name)); err != nil {
		if err := createDesktop(p); err != nil {
			return fmt.Errorf("criar atalho: %w", err)
		}
	}

	// 4. caminho absoluto da HOME isolada
	root := expandHome(profileHome(name))
	if root == "" {
		return errors.New("HOME inválido")
	}

	// 5. ambiente do processo `flatpak`:
	//    HOME + XDG_* → repassados pelo Flatpak para o sandbox.
	env := append(
		os.Environ(),
		"HOME="+root,
		"XDG_CONFIG_HOME="+filepath.Join(root, ".config"),
		"XDG_DATA_HOME="+filepath.Join(root, ".local", "share"),
		"XDG_CACHE_HOME="+filepath.Join(root, ".cache"),
	)

	// 6. argumentos do `flatpak run`:
	//    SDL_* → passados via --env= (o SDL dentro do sandbox precisa vê-los).
	args := []string{
		"run",
		"--env=SDL_JOYSTICK_ALLOW_BACKGROUND_EVENTS=1",
	}

	if ignored := normalizeIgnored(p.IgnoredControllers); len(ignored) > 0 {
		args = append(args,
			"--env=SDL_GAMECONTROLLER_IGNORE_DEVICES="+strings.Join(ignored, ","))
	}

	args = append(args, flatpakID)

	fmt.Println("Executando MCPE:")
	fmt.Println("  HOME             =", root)
	fmt.Println("  XDG_CONFIG_HOME  =", filepath.Join(root, ".config"))
	fmt.Println("  XDG_DATA_HOME    =", filepath.Join(root, ".local", "share"))
	fmt.Println("  XDG_CACHE_HOME   =", filepath.Join(root, ".cache"))
	fmt.Println("  flatpak", strings.Join(args, " "))

	cmd := exec.Command("flatpak", args...)
	cmd.Env = env
	cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin

	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return err
	}

	go func() { _ = cmd.Wait() }()

	return nil
}
