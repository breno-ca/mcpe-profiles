package main

import (
	"errors"
	"os"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type gui struct {
	app fyne.App
	win fyne.Window

	controllers []Controller
	checkRefs   []*widget.Check
	names       []string

	currentName string
	editing     bool

	profileList    *widget.List
	nameEntry      *widget.Entry
	homeLabel      *widget.Label
	ctrlBox        *fyne.Container
	statusLabel    *widget.Label
	refreshCtrlBtn *widget.Button

	editBar   *fyne.Container
	saveBtn   *widget.Button
	cancelBtn *widget.Button
	addBtn    *widget.Button
	renameBtn *widget.Button
	deleteBtn *widget.Button
	runBtn    *widget.Button
}

func runGUI() {
	g := &gui{app: app.NewWithID("com.mcpe.profiles")}
	g.win = g.app.NewWindow("MCPE Profiles")
	g.win.Resize(fyne.NewSize(380, 400))
	g.win.SetFixedSize(false)

	g.refreshControllers()
	g.build()
	g.selectProfile("")
	g.win.ShowAndRun()
}

func (g *gui) showErr(err error) {
	if err != nil {
		dialog.ShowError(err, g.win)
	}
}

func (g *gui) refreshControllers() {
	g.controllers = detectControllers()
}

func (g *gui) reloadNames() {
	g.names = listProfiles()
	if g.profileList != nil {
		g.profileList.Refresh()
	}
}

// ---------- layout ----------

func (g *gui) build() {
	g.addBtn = widget.NewButtonWithIcon("", theme.ContentAddIcon(), g.startNew)
	g.addBtn.Importance = widget.LowImportance

	g.renameBtn = widget.NewButtonWithIcon("", theme.DocumentCreateIcon(), g.onRenameToggle)
	g.renameBtn.Importance = widget.LowImportance

	g.reloadNames()
	g.profileList = widget.NewList(
		func() int { return len(g.names) },
		func() fyne.CanvasObject { return widget.NewLabel("template") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			o.(*widget.Label).SetText(g.names[i])
		},
	)
	g.profileList.OnSelected = func(i widget.ListItemID) {
		if i >= 0 && i < len(g.names) {
			g.selectProfile(g.names[i])
		}
	}

	g.nameEntry = widget.NewEntry()
	g.nameEntry.SetPlaceHolder("nome do perfil")

	g.saveBtn = widget.NewButtonWithIcon("", theme.ConfirmIcon(), g.onSave)
	g.saveBtn.Importance = widget.HighImportance
	g.cancelBtn = widget.NewButtonWithIcon("", theme.CancelIcon(), func() {
		g.setEditing(false)
		g.selectProfile(g.currentName)
	})

	g.editBar = container.NewBorder(
		nil, nil, nil,
		container.NewHBox(g.saveBtn, g.cancelBtn),
		g.nameEntry,
	)
	g.editBar.Hide()

	sidebarHeader := container.NewBorder(
		nil, nil,
		widget.NewLabelWithStyle("Perfis", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewHBox(g.addBtn, g.renameBtn),
		nil,
	)

	sidebar := container.NewBorder(
		container.NewVBox(
			sidebarHeader,
			g.editBar,
			widget.NewSeparator(),
		),
		nil, nil, nil,
		g.profileList,
	)

	g.homeLabel = widget.NewLabel("—")
	g.homeLabel.TextStyle = fyne.TextStyle{Monospace: true}
	g.homeLabel.Truncation = fyne.TextTruncateClip

	g.ctrlBox = container.NewVBox()
	ctrlScroll := container.NewVScroll(g.ctrlBox)
	ctrlScroll.SetMinSize(fyne.NewSize(0, 120))

	g.refreshCtrlBtn = widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
		g.refreshControllers()
		g.populateControllers(g.selectedControllers())
		g.statusLabel.SetText("Controles atualizados.")
	})
	g.refreshCtrlBtn.Importance = widget.LowImportance

	ctrlHeader := container.NewBorder(
		nil, nil,
		widget.NewLabelWithStyle("Controles ignorados", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		g.refreshCtrlBtn,
		nil,
	)

	g.deleteBtn = widget.NewButtonWithIcon("", theme.DeleteIcon(), g.onDelete)
	g.deleteBtn.Importance = widget.DangerImportance

	g.runBtn = widget.NewButtonWithIcon("INICIAR", theme.MediaPlayIcon(), g.onRun)
	g.runBtn.Importance = widget.HighImportance

	g.statusLabel = widget.NewLabel("Selecione ou crie um perfil.")
	g.statusLabel.Alignment = fyne.TextAlignCenter
	g.statusLabel.Truncation = fyne.TextTruncateClip

	rightPanel := container.NewBorder(
		container.NewVBox(
			container.NewBorder(
				nil, nil,
				widget.NewLabel("Home:"),
				nil,
				g.homeLabel,
			),
			widget.NewSeparator(),
			ctrlHeader,
		),
		container.NewVBox(
			g.statusLabel,
			container.NewBorder(
				nil, nil,
				g.deleteBtn,
				g.runBtn,
				widget.NewLabel(""),
			),
		),
		nil, nil,
		ctrlScroll,
	)

	split := container.NewHSplit(
		container.NewPadded(sidebar),
		container.NewPadded(rightPanel),
	)
	split.Offset = 0.38

	g.win.SetContent(split)
	g.populateControllers(nil)
}

func (g *gui) setEditing(on bool) {
	g.editing = on
	if on {
		g.editBar.Show()
		g.nameEntry.Enable()
		g.win.Canvas().Focus(g.nameEntry)
	} else {
		g.editBar.Hide()
		g.nameEntry.Disable()
	}
	g.editBar.Refresh()
}

// ---------- ações ----------

func (g *gui) startNew() {
	g.currentName = ""
	g.profileList.UnselectAll()
	g.nameEntry.SetText("")
	g.nameEntry.SetPlaceHolder("nome do perfil")
	g.homeLabel.SetText("—")
	g.setEditing(true)
	g.populateControllers(nil)
	g.statusLabel.SetText("Digite o nome e confirme.")
}

func (g *gui) selectProfile(name string) {
	if name == "" {
		g.currentName = ""
		g.setEditing(false)
		g.nameEntry.SetText("")
		g.homeLabel.SetText("—")
		g.populateControllers(nil)
		g.statusLabel.SetText("Selecione ou crie um perfil.")
		g.setRightPanelEnabled(false)
		return
	}

	p, err := loadProfile(name)
	if err != nil {
		g.showErr(err)
		return
	}
	g.currentName = name
	g.setEditing(false)
	g.nameEntry.SetText(p.Name)
	g.homeLabel.SetText(p.Home)
	g.populateControllers(p.IgnoredControllers)
	g.statusLabel.SetText("Perfil: " + name)
	g.setRightPanelEnabled(true)
}

func (g *gui) onRenameToggle() {
	if g.currentName == "" {
		g.showErr(errors.New("selecione um perfil primeiro"))
		return
	}
	if g.editing {
		g.setEditing(false)
		g.selectProfile(g.currentName)
		return
	}
	g.nameEntry.SetText(g.currentName)
	g.setEditing(true)
	g.statusLabel.SetText("Edite o nome e confirme para renomear.")
}

func (g *gui) onSave() {
	name := strings.TrimSpace(g.nameEntry.Text)
	if err := validateProfileName(name); err != nil {
		g.showErr(err)
		return
	}

	if g.currentName != "" && g.currentName != name {
		if err := renameProfile(g.currentName, name); err != nil {
			g.showErr(err)
			return
		}
		removeDesktop(g.currentName)
	}

	p := Profile{
		Name:               name,
		Home:               profileHome(name),
		IgnoredControllers: g.selectedControllers(),
	}
	if err := saveProfile(p); err != nil {
		g.showErr(err)
		return
	}
	if err := createDesktop(p); err != nil {
		g.showErr(err)
		return
	}

	g.setEditing(false)
	g.reloadNames()
	g.selectProfile(name)
	g.statusLabel.SetText("Salvo: " + name)
}

func (g *gui) onDelete() {
	if g.currentName == "" {
		g.showErr(errors.New("selecione um perfil primeiro"))
		return
	}
	name := g.currentName
	dialog.ShowConfirm("Excluir perfil",
		"Isso vai apagar DEFINITIVAMENTE:\n\n"+
			"• O perfil \""+name+"\"\n"+
			"• A pasta ~/mcpe-profiles/"+name+" (saves, configs, login)\n\n"+
			"Não tem como desfazer. Continuar?",
		func(ok bool) {
			if !ok {
				return
			}

			// 1. pasta de configuração do perfil
			if err := os.RemoveAll(profilePath(name)); err != nil {
				g.showErr(err)
				return
			}
			// 2. HOME isolada (saves, configs, login)
			if err := os.RemoveAll(expandHome(profileHome(name))); err != nil {
				g.showErr(err)
				return
			}
			// 3. atalho
			removeDesktop(name)

			g.reloadNames()
			g.selectProfile("")
			g.statusLabel.SetText("Excluído: " + name)
		}, g.win)
}

func (g *gui) onRun() {
	if g.currentName == "" {
		g.showErr(errors.New("selecione um perfil primeiro"))
		return
	}
	g.statusLabel.SetText("Iniciando…")
	g.showErr(runProfile(g.currentName))
	g.statusLabel.SetText("")
}

// ---------- controles ----------

func (g *gui) populateControllers(selected []string) {
	g.ctrlBox.RemoveAll()
	g.checkRefs = nil

	want := map[string]bool{}
	for _, k := range selected {
		want[k] = true
	}

	if len(g.controllers) == 0 {
		g.ctrlBox.Add(widget.NewLabel("Nenhum controle detectado."))
	} else {
		for i := range g.controllers {
			c := g.controllers[i]
			chk := widget.NewCheck(c.Label(), nil)
			chk.SetChecked(want[c.Key()])
			g.checkRefs = append(g.checkRefs, chk)
			g.ctrlBox.Add(chk)
		}
	}
	g.ctrlBox.Refresh()

	// reaplica o estado do painel
	g.setRightPanelEnabled(g.currentName != "")
}

func (g *gui) selectedControllers() []string {
	var out []string
	for i, chk := range g.checkRefs {
		if chk.Checked {
			out = append(out, g.controllers[i].Key())
		}
	}
	return out
}

// setRightPanelEnabled liga/desliga todos os widgets interativos do painel direito.
func (g *gui) setRightPanelEnabled(on bool) {
	set := func(w interface {
		Enable()
		Disable()
	},
	) {
		if on {
			w.Enable()
		} else {
			w.Disable()
		}
	}
	set(g.runBtn)
	set(g.deleteBtn)
	set(g.refreshCtrlBtn)
	for _, chk := range g.checkRefs {
		set(chk)
	}
}
