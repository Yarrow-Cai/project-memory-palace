"""Modern desktop GUI for Project Memory Palace.

Features:
- CustomTkinter modern UI with dark/light/system theme
- Bilingual support (Chinese / English)
- MCP server status monitor and start/stop
- Project directory browser
- System tray minimize-to-tray
"""

from __future__ import annotations

import argparse
import subprocess
import sys
import textwrap
import threading
import tkinter as tk
from pathlib import Path
from tkinter import messagebox, ttk
from typing import Any

import customtkinter as ctk

from pmem.constants import MEMORY_STATUSES
from pmem.i18n import get_language, set_language, t as _t
from pmem.service import MemoryNotFoundError, MemoryService

try:
    import pystray
    from PIL import Image, ImageDraw

    _HAS_TRAY = True
except ImportError:
    _HAS_TRAY = False


TYPE_ICONS = {
    "project_goal": "\u2606",
    "design": "\u25c9",
    "decision": "\u2691",
    "change_reason": "\u21bb",
    "bugfix": "\u2620",
    "module": "\u25a3",
    "convention": "\u00a7",
    "open_question": "\u2047",
}

STATUS_ICONS = {
    "active": "\u25cf",
    "stale": "\u25d0",
    "superseded": "\u25cc",
    "rejected": "\u2717",
}

# ── Theme colour tables for non-CTk widgets ────────────────────

DARK_COLORS = {
    "tree_bg": "#2b2b2b",
    "tree_fg": "#dce4ee",
    "tree_field": "#343638",
    "tree_sel_bg": "#1f538d",
    "tree_sel_fg": "#ffffff",
    "tree_heading_bg": "#242424",
    "text_bg": "#343638",
    "text_fg": "#dce4ee",
    "text_field_fg": "#5294e2",
    "text_sep_fg": "#555555",
    "text_empty_fg": "#888888",
}

LIGHT_COLORS = {
    "tree_bg": "#ebebeb",
    "tree_fg": "#1a1a1a",
    "tree_field": "#f5f5f5",
    "tree_sel_bg": "#3b8ed0",
    "tree_sel_fg": "#ffffff",
    "tree_heading_bg": "#d9d9d9",
    "text_bg": "#f5f5f5",
    "text_fg": "#1a1a1a",
    "text_field_fg": "#1f538d",
    "text_sep_fg": "#cccccc",
    "text_empty_fg": "#999999",
}


def _make_tray_image() -> Image.Image:
    img = Image.new("RGBA", (64, 64), (43, 87, 151, 255))
    draw = ImageDraw.Draw(img)
    draw.rounded_rectangle((4, 4, 60, 60), radius=12, fill=(43, 87, 151, 255))
    draw.text((32, 26), "PM", fill=(255, 255, 255, 255), anchor="mm",
              font_size=22)
    return img


class MemoryPalaceApp:
    """Modern desktop GUI for Project Memory Palace."""

    def __init__(self, project_root: Path) -> None:
        self.project_root = project_root.resolve()
        self.service = MemoryService(self.project_root)
        self._cards_cache: dict[str, dict[str, Any]] = {}
        self._mcp_process: subprocess.Popen | None = None
        self.tray_icon: Any = None
        self._tray_thread: threading.Thread | None = None
        self._current_appearance = "dark"

        self._init_project()

        # Appearance
        ctk.set_appearance_mode(self._current_appearance)
        ctk.set_default_color_theme("blue")
        self._colors = DARK_COLORS

        # Root window
        self.root = ctk.CTk()
        self.root.title(f"{_t('app_title')} \u2014 {self.project_root.name}")
        self.root.geometry("1100x680")
        self.root.minsize(800, 480)

        self._build_ui()
        self._setup_tray()
        self.root.protocol("WM_DELETE_WINDOW", self._on_close)
        self.root.after(150, self._load_recent)

    # ── project init ────────────────────────────────────────────

    def _init_project(self) -> None:
        try:
            self.service.init_project()
        except Exception:
            pass

    # ── theme helpers ───────────────────────────────────────────

    def _apply_treeview_style(self) -> None:
        style = ttk.Style()
        style.theme_use("clam")
        c = self._colors
        style.configure("Memory.Treeview",
                        background=c["tree_bg"],
                        foreground=c["tree_fg"],
                        fieldbackground=c["tree_field"],
                        rowheight=30,
                        borderwidth=0)
        style.configure("Memory.Treeview.Heading",
                        background=c["tree_heading_bg"],
                        foreground=c["tree_fg"],
                        font=("Segoe UI", 9, "bold"),
                        borderwidth=0,
                        relief="flat")
        style.map("Memory.Treeview",
                  background=[("selected", c["tree_sel_bg"])],
                  foreground=[("selected", c["tree_sel_fg"])])

    def _apply_text_tags(self) -> None:
        c = self._colors
        for widget in (self.recent_tree, self.search_tree):
            widget.tag_configure("alt",
                                 background=c["tree_field"] if self._current_appearance == "dark" else "#fafafa")
        self.detail_text.tag_configure("field",
                                       foreground=c["text_field_fg"],
                                       font=("Consolas", 11, "bold"))
        self.detail_text.tag_configure("value",
                                       foreground=c["text_fg"],
                                       font=("Consolas", 10))
        self.detail_text.tag_configure("separator",
                                       foreground=c["text_sep_fg"],
                                       font=("Consolas", 10))
        self.detail_text.tag_configure("empty",
                                       foreground=c["text_empty_fg"],
                                       font=("Consolas", 10, "italic"))

    def _toggle_theme(self) -> None:
        if self._current_appearance == "dark":
            self._current_appearance = "light"
            self._colors = LIGHT_COLORS
        else:
            self._current_appearance = "dark"
            self._colors = DARK_COLORS
        ctk.set_appearance_mode(self._current_appearance)
        self._apply_treeview_style()
        self._apply_text_tags()
        self.detail_text.configure(bg=self._colors["text_bg"],
                                   fg=self._colors["text_fg"])
        self._update_theme_button()
        if hasattr(self, '_current_detail'):
            self._render_detail(self._current_detail)

    def _update_theme_button(self) -> None:
        icon = "\u2600" if self._current_appearance == "dark" else "\ud83c\udf19"
        self.theme_btn.configure(text=icon)

    # ── language ────────────────────────────────────────────────

    def _toggle_language(self) -> None:
        current = get_language()
        new_lang = "zh" if current == "en" else "en"
        set_language(new_lang)
        self._refresh_ui_texts()
        if hasattr(self, '_current_detail'):
            self._render_detail(self._current_detail)

    def _refresh_ui_texts(self) -> None:
        self.root.title(f"{_t('app_title')} \u2014 {self.project_root.name}")
        self.lang_btn.configure(text="EN" if get_language() == "zh" else "\u4e2d\u6587")
        self.search_entry.configure(placeholder_text=_t("search_placeholder"))
        self.search_btn.configure(text=_t("search"))
        self.tabview._segmented_button._buttons[0].configure(text=_t("recent"))
        self.tabview._segmented_button._buttons[1].configure(text=_t("search_results"))
        self.browse_btn.configure(text=_t("browse"))
        self.project_label.configure(text=_t("project_label") + ":")
        if self._selected_id is None:
            self.detail_title_label.configure(text=_t("select_memory"))
        self._update_mcp_ui()

    # ── MCP server ──────────────────────────────────────────────

    @property
    def _mcp_available(self) -> bool:
        try:
            import pmem.mcp_server  # noqa: F401
            return True
        except ImportError:
            return False

    @property
    def _mcp_running(self) -> bool:
        return self._mcp_process is not None and self._mcp_process.poll() is None

    def _toggle_mcp(self) -> None:
        if self._mcp_running:
            self._stop_mcp()
        elif self._mcp_available:
            self._start_mcp()

    def _start_mcp(self) -> None:
        try:
            self._mcp_process = subprocess.Popen(
                [sys.executable, "-m", "pmem.mcp_server"],
                cwd=str(self.project_root),
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
            )
            self._update_mcp_ui()
        except Exception as exc:
            self._set_status(f"MCP start error: {exc}")

    def _stop_mcp(self) -> None:
        if self._mcp_process:
            self._mcp_process.terminate()
            self._mcp_process = None
            self._update_mcp_ui()

    def _update_mcp_ui(self) -> None:
        if not self._mcp_available:
            self.mcp_indicator.configure(text="\u26ab", text_color="#888888")
            self.mcp_label.configure(text="MCP N/A", text_color="#888888")
            self.mcp_btn.configure(state="disabled", text=_t("mcp_start"))
            return

        if self._mcp_running:
            self.mcp_indicator.configure(text="\U0001f7e2", text_color=None)
            self.mcp_label.configure(text=_t("mcp_running"), text_color=None)
            self.mcp_btn.configure(text=_t("mcp_stop"))
        else:
            self.mcp_indicator.configure(text="\U0001f534", text_color=None)
            self.mcp_label.configure(text=_t("mcp_stopped"), text_color=None)
            self.mcp_btn.configure(text=_t("mcp_start"))

    # ── UI construction ─────────────────────────────────────────

    def _build_ui(self) -> None:
        self._apply_treeview_style()
        self._build_title_bar()
        self._build_project_bar()
        self._build_search_bar()
        self._build_main_panel()
        self._build_status_bar()
        self._apply_text_tags()

    def _build_title_bar(self) -> None:
        bar = ctk.CTkFrame(self.root, height=44, corner_radius=0, fg_color="transparent")
        bar.pack(fill=tk.X, padx=16, pady=(10, 0))

        title = ctk.CTkLabel(bar, text="\U0001f3f0  Project Memory Palace",
                             font=ctk.CTkFont(size=16, weight="bold"))
        title.pack(side=tk.LEFT)

        self.theme_btn = ctk.CTkButton(
            bar, text="", width=36, height=32, fg_color="transparent",
            border_width=1, border_color=("gray40", "gray60"),
            command=self._toggle_theme,
        )
        self.theme_btn.pack(side=tk.RIGHT, padx=(4, 0))
        self._update_theme_button()

        self.lang_btn = ctk.CTkButton(
            bar, text="\u4e2d\u6587", width=50, height=32,
            fg_color="transparent", border_width=1,
            border_color=("gray40", "gray60"),
            command=self._toggle_language,
        )
        self.lang_btn.pack(side=tk.RIGHT, padx=(4, 0))

        # MCP section
        mcp_frame = ctk.CTkFrame(bar, fg_color="transparent")
        mcp_frame.pack(side=tk.RIGHT, padx=(8, 0))

        self.mcp_indicator = ctk.CTkLabel(mcp_frame, text="\u26ab",
                                          font=ctk.CTkFont(size=12), width=20)
        self.mcp_indicator.pack(side=tk.LEFT)

        self.mcp_label = ctk.CTkLabel(mcp_frame, text="MCP",
                                      font=ctk.CTkFont(size=11))
        self.mcp_label.pack(side=tk.LEFT, padx=(2, 6))

        self.mcp_btn = ctk.CTkButton(
            mcp_frame, text=_t("mcp_start"), width=90, height=28,
            font=ctk.CTkFont(size=11), command=self._toggle_mcp,
        )
        self.mcp_btn.pack(side=tk.LEFT)
        self._update_mcp_ui()

    def _build_project_bar(self) -> None:
        bar = ctk.CTkFrame(self.root, height=38, corner_radius=6, fg_color="transparent")
        bar.pack(fill=tk.X, padx=16, pady=(6, 0))

        self.project_label = ctk.CTkLabel(bar, text=_t("project_label") + ":",
                                          font=ctk.CTkFont(size=11))
        self.project_label.pack(side=tk.LEFT, padx=(0, 6))

        self.project_entry = ctk.CTkEntry(bar, height=30, font=ctk.CTkFont(size=11))
        self.project_entry.insert(0, str(self.project_root))
        self.project_entry.pack(side=tk.LEFT, fill=tk.X, expand=True, padx=(0, 6))
        self.project_entry.bind("<Return>", lambda _e: self._apply_project())

        self.browse_btn = ctk.CTkButton(
            bar, text=_t("browse"), width=70, height=30,
            font=ctk.CTkFont(size=11), command=self._browse_project,
        )
        self.browse_btn.pack(side=tk.LEFT)

    def _build_search_bar(self) -> None:
        bar = ctk.CTkFrame(self.root, height=38, corner_radius=6, fg_color="transparent")
        bar.pack(fill=tk.X, padx=16, pady=(6, 6))

        self.search_entry = ctk.CTkEntry(bar, height=34,
                                         placeholder_text=_t("search_placeholder"),
                                         font=ctk.CTkFont(size=12))
        self.search_entry.pack(side=tk.LEFT, fill=tk.X, expand=True, padx=(0, 8))
        self.search_entry.bind("<Return>", lambda _e: self._search())

        self.search_btn = ctk.CTkButton(
            bar, text=_t("search"), width=80, height=34,
            font=ctk.CTkFont(size=12), command=self._search,
        )
        self.search_btn.pack(side=tk.LEFT)

        refresh_btn = ctk.CTkButton(
            bar, text="\u21bb", width=36, height=34,
            font=ctk.CTkFont(size=14), fg_color="transparent",
            border_width=1, border_color=("gray40", "gray60"),
            command=self._refresh,
        )
        refresh_btn.pack(side=tk.LEFT, padx=(6, 0))

    def _build_main_panel(self) -> None:
        main = ctk.CTkFrame(self.root, corner_radius=8)
        main.pack(fill=tk.BOTH, expand=True, padx=16, pady=4)

        # Tab view
        self.tabview = ctk.CTkTabview(main, corner_radius=6)
        self.tabview.pack(fill=tk.BOTH, expand=True, padx=8, pady=(8, 0))

        self.tabview.add(_t("recent"))
        self.tabview.add(_t("search_results"))

        recent_tab = self.tabview.tab(_t("recent"))
        search_tab = self.tabview.tab(_t("search_results"))

        # Paned content
        paned = ttk.PanedWindow(main, orient=tk.HORIZONTAL)
        paned.pack(fill=tk.BOTH, expand=True, padx=8, pady=8)

        # Treeview panel
        tree_container = ctk.CTkFrame(paned, fg_color="transparent")
        paned.add(tree_container, weight=3)

        self.recent_tree = self._make_treeview(tree_container)
        self.search_tree = self._make_treeview(tree_container)

        # Switch tree based on tab
        def _on_tab_change():
            cur = self.tabview.get()
            self.recent_tree.pack_forget()
            self.search_tree.pack_forget()
            if cur == _t("recent"):
                self.recent_tree.pack(fill=tk.BOTH, expand=True)
            else:
                self.search_tree.pack(fill=tk.BOTH, expand=True)
        self.tabview.configure(command=_on_tab_change)
        self.recent_tree.pack(fill=tk.BOTH, expand=True)

        # Detail panel
        detail_container = ctk.CTkFrame(paned, fg_color="transparent")
        paned.add(detail_container, weight=4)

        header_frame = ctk.CTkFrame(detail_container, fg_color="transparent")
        header_frame.pack(fill=tk.X, pady=(0, 6))

        self.detail_title_label = ctk.CTkLabel(
            header_frame, text=_t("select_memory"),
            font=ctk.CTkFont(size=14, weight="bold"),
            wraplength=380, justify="left",
        )
        self.detail_title_label.pack(anchor=tk.W)

        self.detail_meta_label = ctk.CTkLabel(
            header_frame, text="",
            font=ctk.CTkFont(size=11),
            text_color=("gray50", "gray60"),
        )
        self.detail_meta_label.pack(anchor=tk.W, pady=(3, 0))

        text_frame = ctk.CTkFrame(detail_container, corner_radius=6)
        text_frame.pack(fill=tk.BOTH, expand=True)

        self.detail_text = tk.Text(
            text_frame, wrap=tk.WORD, font=("Consolas", 10),
            bg=self._colors["text_bg"], fg=self._colors["text_fg"],
            borderwidth=0, padx=14, pady=12, state=tk.DISABLED,
            relief=tk.FLAT,
        )
        detail_scroll = ctk.CTkScrollbar(text_frame, orientation="vertical",
                                         command=self.detail_text.yview)
        self.detail_text.configure(yscrollcommand=detail_scroll.set)
        self.detail_text.pack(side=tk.LEFT, fill=tk.BOTH, expand=True)
        detail_scroll.pack(side=tk.RIGHT, fill=tk.Y)

        self._selected_id = None
        self._current_detail = None

    def _make_treeview(self, parent: ctk.CTkFrame) -> ttk.Treeview:
        columns = ("status", "type", "title", "updated")
        tree = ttk.Treeview(parent, columns=columns, show="headings",
                            selectmode="browse", style="Memory.Treeview")
        tree.heading("status", text="", anchor=tk.CENTER)
        tree.heading("type", text="", anchor=tk.CENTER)
        tree.heading("title", text=_t("title"), anchor=tk.W)
        tree.heading("updated", text=_t("updated"), anchor=tk.W)

        tree.column("status", width=32, anchor=tk.CENTER, stretch=False)
        tree.column("type", width=36, anchor=tk.CENTER, stretch=False)
        tree.column("title", width=280, anchor=tk.W)
        tree.column("updated", width=130, anchor=tk.W, stretch=False)

        tree.bind("<<TreeviewSelect>>", self._on_tree_select)
        tree.bind("<ButtonRelease-3>", self._on_right_click)
        tree.tag_configure("alt", background=self._colors["tree_field"])

        return tree

    def _build_status_bar(self) -> None:
        bar = ctk.CTkFrame(self.root, height=28, corner_radius=0, fg_color="transparent")
        bar.pack(fill=tk.X, padx=20, pady=(2, 8))

        self.status_label = ctk.CTkLabel(bar, text=_t("ready"),
                                         font=ctk.CTkFont(size=10),
                                         text_color=("gray50", "gray60"))
        self.status_label.pack(side=tk.LEFT)

        self.status_count = ctk.CTkLabel(bar, text="",
                                         font=ctk.CTkFont(size=10),
                                         text_color=("gray50", "gray60"))
        self.status_count.pack(side=tk.RIGHT)

        version_label = ctk.CTkLabel(bar, text="v0.1.0",
                                     font=ctk.CTkFont(size=10),
                                     text_color=("gray50", "gray60"))
        version_label.pack(side=tk.RIGHT, padx=(0, 12))

    # ── system tray ─────────────────────────────────────────────

    def _setup_tray(self) -> None:
        if not _HAS_TRAY:
            return
        try:
            image = _make_tray_image()
            menu = pystray.Menu(
                pystray.MenuItem(_t("show"), self._show_window, default=True),
                pystray.Menu.SEPARATOR,
                pystray.MenuItem(_t("exit"), self._quit_app),
            )
            self.tray_icon = pystray.Icon(
                "pmem", image, _t("app_title"), menu,
            )
        except Exception:
            self.tray_icon = None

    def _start_tray(self) -> None:
        if self.tray_icon is None:
            return
        self._tray_thread = threading.Thread(target=self.tray_icon.run, daemon=True)
        self._tray_thread.start()

    def _stop_tray(self) -> None:
        if self.tray_icon is not None:
            self.tray_icon.stop()

    def _show_window(self, _icon: Any = None, _item: Any = None) -> None:
        self.root.after(0, self._restore_window)

    def _restore_window(self) -> None:
        self.root.deiconify()
        self.root.lift()
        self.root.focus_force()

    def _on_close(self) -> None:
        if _HAS_TRAY and self.tray_icon is not None:
            self.root.withdraw()
        else:
            self._quit_app()

    def _quit_app(self) -> None:
        self._stop_mcp()
        self._stop_tray()
        self.root.destroy()

    # ── project browsing ────────────────────────────────────────

    def _browse_project(self) -> None:
        from tkinter import filedialog
        new_root = filedialog.askdirectory(title=_t("open_project"))
        if not new_root:
            return
        new_path = Path(new_root).resolve()
        try:
            svc = MemoryService(new_path)
            svc.init_project()
        except Exception as exc:
            messagebox.showerror(_t("error"), f"Cannot open project: {exc}")
            return
        self.project_root = new_path
        self.service = svc
        self._cards_cache.clear()
        self.project_entry.delete(0, tk.END)
        self.project_entry.insert(0, str(new_path))
        self.root.title(f"{_t('app_title')} \u2014 {self.project_root.name}")
        self._clear_detail()
        self._load_recent()

    def _apply_project(self) -> None:
        new_path = Path(self.project_entry.get()).resolve()
        if new_path == self.project_root:
            return
        try:
            svc = MemoryService(new_path)
            svc.init_project()
        except Exception as exc:
            self._set_status(f"{_t('error')}: {exc}")
            return
        self.project_root = new_path
        self.service = svc
        self._cards_cache.clear()
        self.root.title(f"{_t('app_title')} \u2014 {self.project_root.name}")
        self._clear_detail()
        self._load_recent()

    # ── data loading ────────────────────────────────────────────

    def _load_recent(self) -> None:
        self._set_status(_t("loading"))
        try:
            results = self.service.list_recent(50)
        except Exception as exc:
            self._set_status(f"{_t('error')}: {exc}")
            return
        self._populate_tree(self.recent_tree, results)
        self._set_status(f"{len(results)} {_t('memories')}")

    def _search(self) -> None:
        query = self.search_entry.get().strip()
        if not query:
            return
        self._set_status(_t("searching"))
        try:
            results = self.service.recall(query, {}, 30)
        except Exception as exc:
            self._set_status(f"{_t('error')}: {exc}")
            return
        self._populate_tree(self.search_tree, results)
        self._set_status(_t("results_for").format(query=query))
        # Switch to search tab
        self.tabview.set(_t("search_results"))
        self.search_tree.pack(fill=tk.BOTH, expand=True)
        self.recent_tree.pack_forget()

    def _refresh(self) -> None:
        cur = self.tabview.get()
        if cur == _t("recent"):
            self._load_recent()
        else:
            q = self.search_entry.get().strip()
            if q:
                self._search()
            else:
                self._load_recent()

    def _rebuild_index(self) -> None:
        self._set_status(_t("rebuilding"))
        try:
            self.service.rebuild_index()
        except Exception as exc:
            self._set_status(f"{_t('error')}: {exc}")
            return
        self._set_status(_t("rebuilt"))
        self._refresh()

    # ── tree population ─────────────────────────────────────────

    def _populate_tree(self, tree: ttk.Treeview,
                       rows: list[dict[str, Any]]) -> None:
        for item in tree.get_children():
            tree.delete(item)
        for idx, row in enumerate(rows):
            sid = STATUS_ICONS.get(row["status"], "\u25cf")
            type_icon = TYPE_ICONS.get(row["type"], "\u25cf")
            updated_short = row["updated_at"][:16].replace("T", " ")
            tag = "alt" if idx % 2 == 1 else ""
            tree.insert(
                "", tk.END, iid=row["id"],
                values=(sid, type_icon, row["title"], updated_short),
                tags=(tag,),
            )
        self.status_count.configure(text=f"{len(rows)} {_t('items')}")

    def _on_tree_select(self, event: Any) -> None:
        tree: ttk.Treeview = event.widget
        selection = tree.selection()
        if not selection:
            return
        self._show_detail(selection[0])

    def _on_right_click(self, event: Any) -> None:
        tree: ttk.Treeview = event.widget
        item = tree.identify_row(event.y)
        if not item:
            return
        tree.selection_set(item)
        self._show_detail(item)
        menu = tk.Menu(self.root, tearoff=0, font=("Segoe UI", 10))
        menu.add_command(label=_t("copy_id"),
                         command=lambda i=item: self._copy_id(i))
        menu.add_separator()
        for status in sorted(MEMORY_STATUSES):
            menu.add_command(
                label=f"{_t('mark_as')} {status}",
                command=lambda s=status, i=item: self._update_status(i, s),
            )
        menu.tk_popup(event.x_root, event.y_root)

    def _copy_id(self, memory_id: str) -> None:
        self.root.clipboard_clear()
        self.root.clipboard_append(memory_id)
        self._set_status(f"{_t('copied')}: {memory_id}")

    def _update_status(self, memory_id: str, status: str) -> None:
        try:
            self.service.update_memory(memory_id, {"status": status})
            self._cards_cache.pop(memory_id, None)
            self._set_status(_t("updated_to").format(id=memory_id, status=status))
            self._refresh()
            if self._selected_id == memory_id:
                self._show_detail(memory_id)
        except Exception as exc:
            messagebox.showerror(f"{_t('error')}", str(exc))

    # ── detail rendering ────────────────────────────────────────

    def _show_detail(self, memory_id: str) -> None:
        self._selected_id = memory_id
        try:
            data = self.service.open_memory(memory_id)
        except MemoryNotFoundError:
            self._set_status(f"Memory not found: {memory_id}")
            return
        except Exception as exc:
            self._set_status(f"{_t('error')}: {exc}")
            return
        self._cards_cache[memory_id] = data
        self._current_detail = data
        self._render_detail(data)

    def _render_detail(self, data: dict[str, Any]) -> None:
        self.detail_text.configure(state=tk.NORMAL)
        self.detail_text.delete("1.0", tk.END)

        self.detail_title_label.configure(text=data["title"])

        status = data["status"]
        icon = STATUS_ICONS.get(status, "\u25cf")
        meta = (
            f"{_t('id_label')}: {data['id']}  \u2022  "
            f"{_t('type')}: {data['type']}  \u2022  "
            f"{_t('status')}: {icon} {status}  \u2022  "
            f"{_t('confidence')}: {data['confidence']:.0%}"
        )
        self.detail_meta_label.configure(text=meta)

        spacer = "\u2500" * 56

        sections: list[tuple[str, str | None]] = [
            (_t("detail_summary"), data.get("summary")),
            (_t("detail_content"), data.get("content")),
        ]

        for label, value in sections:
            self.detail_text.insert(tk.END, f"{label}\n", "field")
            self.detail_text.insert(tk.END, f"{spacer}\n", "separator")
            if value:
                wrapped = textwrap.fill(value, width=78)
                self.detail_text.insert(tk.END, f"{wrapped}\n\n", "value")
            else:
                self.detail_text.insert(tk.END, f"{_t('empty')}\n\n", "empty")

        source = data.get("source", {})
        self.detail_text.insert(tk.END, f"{_t('detail_source')}\n", "field")
        self.detail_text.insert(tk.END, f"{spacer}\n", "separator")
        src_text = f"Kind: {source.get('kind', '?')}\n{source.get('description', '')}"
        self.detail_text.insert(tk.END, f"{src_text}\n\n", "value")

        tags = data.get("tags", [])
        self.detail_text.insert(tk.END, f"{_t('detail_tags')}\n", "field")
        self.detail_text.insert(tk.END, f"{spacer}\n", "separator")
        if tags:
            self.detail_text.insert(tk.END, ", ".join(tags) + "\n\n", "value")
        else:
            self.detail_text.insert(tk.END, f"{_t('none')}\n\n", "empty")

        scope = data.get("scope", {})
        self.detail_text.insert(tk.END, f"{_t('detail_scope')}\n", "field")
        self.detail_text.insert(tk.END, f"{spacer}\n", "separator")
        scope_parts = []
        if scope.get("project"):
            scope_parts.append(f"Project: {scope['project']}")
        if scope.get("modules"):
            scope_parts.append(f"Modules: {', '.join(scope['modules'])}")
        if scope.get("paths"):
            scope_parts.append(f"Paths: {', '.join(scope['paths'])}")
        self.detail_text.insert(
            tk.END,
            ("\n".join(scope_parts) if scope_parts else f"{_t('none')}") + "\n\n",
            "value" if scope_parts else "empty",
        )

        relations = data.get("relations", {})
        self.detail_text.insert(tk.END, f"{_t('detail_relations')}\n", "field")
        self.detail_text.insert(tk.END, f"{spacer}\n", "separator")
        rel_parts = []
        for rel, targets in relations.items():
            if targets:
                rel_parts.append(f"{rel}: {', '.join(targets)}")
        self.detail_text.insert(
            tk.END,
            ("\n".join(rel_parts) if rel_parts else f"{_t('none')}") + "\n\n",
            "value" if rel_parts else "empty",
        )

        self.detail_text.configure(state=tk.DISABLED)

    def _clear_detail(self) -> None:
        self._selected_id = None
        self._current_detail = None
        self.detail_title_label.configure(text=_t("select_memory"))
        self.detail_meta_label.configure(text="")
        self.detail_text.configure(state=tk.NORMAL)
        self.detail_text.delete("1.0", tk.END)
        self.detail_text.configure(state=tk.DISABLED)

    # ── helpers ─────────────────────────────────────────────────

    def _set_status(self, text: str) -> None:
        self.status_label.configure(text=text)

    # ── run ─────────────────────────────────────────────────────

    def run(self) -> None:
        self._start_tray()
        self.root.mainloop()


def main() -> None:
    parser = argparse.ArgumentParser(
        prog="pmem-gui",
        description="Modern desktop GUI for Project Memory Palace",
    )
    parser.add_argument(
        "--project-root", default=".",
        help="Project root directory (default: current directory)",
    )
    args = parser.parse_args()
    app = MemoryPalaceApp(Path(args.project_root))
    app.run()


if __name__ == "__main__":
    main()