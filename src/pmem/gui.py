from __future__ import annotations

import argparse
import textwrap
import threading
import tkinter as tk
from pathlib import Path
from tkinter import messagebox, scrolledtext, ttk
from typing import Any

from pmem.constants import MEMORY_STATUSES
from pmem.service import MemoryNotFoundError, MemoryService

try:
    import pystray
    from PIL import Image, ImageDraw

    _HAS_TRAY = True
except ImportError:
    _HAS_TRAY = False


BG = "#f5f5f5"
ACCENT = "#2b5797"
ACCENT_LIGHT = "#e8eff7"
TEXT = "#1a1a1a"
TEXT_SECONDARY = "#666666"
BORDER = "#d0d0d0"
ROW_ALT = "#fafafa"
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


def _status_icon(status):
    icons = {"active": "\u25cf", "stale": "\u25d0", "superseded": "\u25cc", "rejected": "\u2717"}
    return icons.get(status, "\u25cf")


def _make_tray_image():
    img = Image.new("RGBA", (64, 64), (43, 87, 151, 255))
    draw = ImageDraw.Draw(img)
    draw.rounded_rectangle((4, 4, 60, 60), radius=12, fill=(43, 87, 151, 255))
    draw.text((32, 26), "PM", fill=(255, 255, 255, 255), anchor="mm", font_size=22)
    return img


class MemoryPalaceApp:
    """Desktop GUI for browsing and searching the project memory palace."""

    def __init__(self, project_root):
        self.project_root = project_root.resolve()
        self.service = MemoryService(self.project_root)
        self.tray_icon = None
        self._tray_thread = None
        self._cards_cache = {}
        self._selected_id = None

        self._init_project()

        self.root = tk.Tk()
        self.root.title(f"Project Memory Palace \u2014 {self.project_root.name}")
        self.root.geometry("1050x650")
        self.root.minsize(720, 420)
        self.root.configure(bg=BG)

        self._build_style()
        self._build_menu()
        self._build_search_bar()
        self._build_main_panel()
        self._build_status_bar()
        self._setup_tray()

        self.root.protocol("WM_DELETE_WINDOW", self._on_close)
        self.root.after(100, self._load_recent)

    def _init_project(self):
        try:
            self.service.init_project()
        except Exception:
            pass

    def _build_style(self):
        style = ttk.Style()
        style.theme_use("clam")
        style.configure(".", background=BG, foreground=TEXT, fieldbackground="white")
        style.configure("TFrame", background=BG)
        style.configure("TLabel", background=BG, foreground=TEXT)
        style.configure("Accent.TButton", background=ACCENT, foreground="white")
        style.map("Accent.TButton",
                  background=[("active", "#1f4478"), ("disabled", "#a0a0a0")])
        style.configure("Treeview",
                        background="white",
                        fieldbackground="white",
                        foreground=TEXT,
                        rowheight=28,
                        borderwidth=0)
        style.configure("Treeview.Heading",
                        background="#eaeaea",
                        foreground=TEXT_SECONDARY,
                        font=("Segoe UI", 9, "bold"),
                        borderwidth=0)
        style.map("Treeview",
                  background=[("selected", ACCENT_LIGHT)],
                  foreground=[("selected", TEXT)])
        style.configure("TNotebook", background=BG, borderwidth=0)
        style.configure("TNotebook.Tab", padding=(16, 6))
        style.map("TNotebook.Tab",
                  background=[("selected", "white")],
                  foreground=[("selected", ACCENT)])
        style.configure("Small.TLabel", font=("Segoe UI", 9), foreground=TEXT_SECONDARY)
        style.configure("Heading.TLabel", font=("Segoe UI", 10, "bold"), foreground=TEXT)
        style.configure("Accent.TLabel", font=("Segoe UI", 10, "bold"), foreground=ACCENT)

    def _build_menu(self):
        menubar = tk.Menu(self.root, font=("Segoe UI", 9))
        file_menu = tk.Menu(menubar, tearoff=0)
        file_menu.add_command(label="Refresh Index", command=self._rebuild_index)
        file_menu.add_command(label="Open Project...", command=self._open_project)
        file_menu.add_separator()
        file_menu.add_command(label="Exit", command=self._quit_app)
        menubar.add_cascade(label="File", menu=file_menu)

        view_menu = tk.Menu(menubar, tearoff=0)
        view_menu.add_command(label="Refresh List", command=self._refresh)
        view_menu.add_separator()
        self._show_tray_var = tk.BooleanVar(value=_HAS_TRAY)
        if _HAS_TRAY:
            view_menu.add_checkbutton(
                label="Minimize to System Tray",
                variable=self._show_tray_var,
            )
        menubar.add_cascade(label="View", menu=view_menu)
        self.root.config(menu=menubar)

    def _build_search_bar(self):
        frame = ttk.Frame(self.root)
        frame.pack(fill=tk.X, padx=12, pady=(10, 4))

        icon_label = ttk.Label(frame, text="\U0001f50d", font=("Segoe UI", 11))
        icon_label.pack(side=tk.LEFT, padx=(0, 8))

        self.search_var = tk.StringVar()
        self.search_entry = ttk.Entry(frame, textvariable=self.search_var,
                                      font=("Segoe UI", 10))
        self.search_entry.pack(side=tk.LEFT, fill=tk.X, expand=True, padx=(0, 6))
        self.search_entry.bind("<Return>", lambda _e: self._search())

        search_btn = ttk.Button(frame, text="Search", style="Accent.TButton",
                                command=self._search)
        search_btn.pack(side=tk.LEFT)

    def _build_main_panel(self):
        paned = ttk.PanedWindow(self.root, orient=tk.HORIZONTAL)
        paned.pack(fill=tk.BOTH, expand=True, padx=12, pady=(4, 8))

        left = ttk.Frame(paned)
        paned.add(left, weight=3)

        self.notebook = ttk.Notebook(left)
        self.notebook.pack(fill=tk.BOTH, expand=True)

        recent_frame = ttk.Frame(self.notebook)
        self.notebook.add(recent_frame, text="Recent")
        self.recent_tree = self._make_treeview(recent_frame)

        search_frame = ttk.Frame(self.notebook)
        self.notebook.add(search_frame, text="Search Results")
        self.search_tree = self._make_treeview(search_frame)

        right = ttk.Frame(paned)
        paned.add(right, weight=4)

        header_frame = ttk.Frame(right)
        header_frame.pack(fill=tk.X, pady=(0, 6))
        self.detail_title_label = ttk.Label(
            header_frame, text="Select a memory",
            style="Heading.TLabel", wraplength=380,
        )
        self.detail_title_label.pack(anchor=tk.W)
        self.detail_meta_label = ttk.Label(
            header_frame, text="",
            style="Small.TLabel",
        )
        self.detail_meta_label.pack(anchor=tk.W, pady=(2, 0))

        self.detail_text = tk.Text(
            right,
            wrap=tk.WORD,
            font=("Consolas", 10),
            bg="white",
            fg=TEXT,
            borderwidth=1,
            relief=tk.SOLID,
            padx=12,
            pady=10,
            state=tk.DISABLED,
        )
        detail_scroll = ttk.Scrollbar(right, orient=tk.VERTICAL,
                                      command=self.detail_text.yview)
        self.detail_text.configure(yscrollcommand=detail_scroll.set)
        self.detail_text.pack(side=tk.LEFT, fill=tk.BOTH, expand=True)
        detail_scroll.pack(side=tk.RIGHT, fill=tk.Y)

        self.detail_text.tag_configure("field", foreground=ACCENT,
                                       font=("Consolas", 10, "bold"))
        self.detail_text.tag_configure("value", foreground=TEXT,
                                       font=("Consolas", 10))
        self.detail_text.tag_configure("separator", foreground=BORDER,
                                       font=("Consolas", 10))
        self.detail_text.tag_configure("empty", foreground=TEXT_SECONDARY,
                                       font=("Consolas", 10, "italic"))

    def _make_treeview(self, parent):
        columns = ("status", "type", "title", "updated")
        tree = ttk.Treeview(parent, columns=columns, show="headings",
                            selectmode="browse")
        tree.heading("status", text="", anchor=tk.CENTER)
        tree.heading("type", text="Type", anchor=tk.CENTER)
        tree.heading("title", text="Title", anchor=tk.W)
        tree.heading("updated", text="Updated", anchor=tk.W)

        tree.column("status", width=28, anchor=tk.CENTER, stretch=False)
        tree.column("type", width=72, anchor=tk.CENTER, stretch=False)
        tree.column("title", width=260, anchor=tk.W)
        tree.column("updated", width=130, anchor=tk.W, stretch=False)

        scrollbar = ttk.Scrollbar(parent, orient=tk.VERTICAL, command=tree.yview)
        tree.configure(yscrollcommand=scrollbar.set)

        tree.pack(side=tk.LEFT, fill=tk.BOTH, expand=True)
        scrollbar.pack(side=tk.RIGHT, fill=tk.Y)

        tree.bind("<<TreeviewSelect>>", self._on_tree_select)
        tree.bind("<ButtonRelease-3>", self._on_right_click)

        tree.tag_configure("alt", background=ROW_ALT)

        return tree

    def _build_status_bar(self):
        frame = ttk.Frame(self.root)
        frame.pack(fill=tk.X, padx=12, pady=(0, 6))
        self.status_label = ttk.Label(frame, text="Ready", style="Small.TLabel")
        self.status_label.pack(side=tk.LEFT)
        self.status_count = ttk.Label(frame, text="", style="Small.TLabel")
        self.status_count.pack(side=tk.RIGHT)

    def _setup_tray(self):
        if not _HAS_TRAY:
            return
        try:
            image = _make_tray_image()
            menu = pystray.Menu(
                pystray.MenuItem("Show", self._show_window, default=True),
                pystray.Menu.SEPARATOR,
                pystray.MenuItem("Exit", self._quit_app),
            )
            self.tray_icon = pystray.Icon(
                "pmem", image, "Project Memory Palace", menu,
            )
        except Exception:
            self.tray_icon = None

    def _start_tray(self):
        if self.tray_icon is None:
            return
        self._tray_thread = threading.Thread(target=self.tray_icon.run, daemon=True)
        self._tray_thread.start()

    def _stop_tray(self):
        if self.tray_icon is not None:
            self.tray_icon.stop()

    def _show_window(self, _icon=None, _item=None):
        self.root.after(0, self._restore_window)

    def _restore_window(self):
        self.root.deiconify()
        self.root.lift()
        self.root.focus_force()

    def _on_close(self):
        if _HAS_TRAY and self._show_tray_var.get() and self.tray_icon is not None:
            self.root.withdraw()
        else:
            self._quit_app()

    def _quit_app(self):
        self._stop_tray()
        self.root.destroy()

    def _load_recent(self):
        self._set_status("Loading recent memories...")
        try:
            results = self.service.list_recent(50)
        except Exception as exc:
            self._set_status(f"Error: {exc}")
            return
        self._populate_tree(self.recent_tree, results)
        self._set_status(f"{len(results)} recent memories")
        self.notebook.select(0)

    def _search(self):
        query = self.search_var.get().strip()
        if not query:
            return
        self._set_status(f"Searching: {query}...")
        try:
            results = self.service.recall(query, {}, 30)
        except Exception as exc:
            self._set_status(f"Error: {exc}")
            return
        self._populate_tree(self.search_tree, results)
        self._set_status(f'{len(results)} results for "{query}"')
        self.notebook.select(1)

    def _refresh(self):
        tab = self.notebook.index("current")
        if tab == 0:
            self._load_recent()
        elif tab == 1:
            q = self.search_var.get().strip()
            if q:
                self._search()
            else:
                self._load_recent()

    def _rebuild_index(self):
        self._set_status("Rebuilding index...")
        try:
            self.service.rebuild_index()
        except Exception as exc:
            self._set_status(f"Error: {exc}")
            return
        self._set_status("Index rebuilt")
        self._refresh()

    def _open_project(self):
        from tkinter import filedialog
        new_root = filedialog.askdirectory(title="Select Project Root")
        if not new_root:
            return
        new_path = Path(new_root).resolve()
        try:
            svc = MemoryService(new_path)
            svc.init_project()
        except Exception as exc:
            messagebox.showerror("Error", f"Cannot open project: {exc}")
            return
        self.project_root = new_path
        self.service = svc
        self._cards_cache.clear()
        self.root.title(f"Project Memory Palace \u2014 {self.project_root.name}")
        self._clear_detail()
        self._load_recent()

    def _populate_tree(self, tree, rows):
        for item in tree.get_children():
            tree.delete(item)
        for idx, row in enumerate(rows):
            sid = _status_icon(row["status"])
            type_icon = TYPE_ICONS.get(row["type"], "\u25cf")
            updated_short = row["updated_at"][:16].replace("T", " ")
            tag = "alt" if idx % 2 == 1 else ""
            tree.insert(
                "",
                tk.END,
                iid=row["id"],
                values=(sid, type_icon, row["title"], updated_short),
                tags=(tag,),
            )
        self.status_count.configure(text=f"{len(rows)} items")

    def _on_tree_select(self, event):
        tree = event.widget
        selection = tree.selection()
        if not selection:
            return
        memory_id = selection[0]
        self._show_detail(memory_id)

    def _on_right_click(self, event):
        tree = event.widget
        item = tree.identify_row(event.y)
        if not item:
            return
        tree.selection_set(item)
        self._show_detail(item)
        menu = tk.Menu(self.root, tearoff=0, font=("Segoe UI", 9))
        menu.add_command(label="Copy ID",
                         command=lambda i=item: self._copy_id(i))
        menu.add_separator()
        for status in sorted(MEMORY_STATUSES):
            menu.add_command(
                label=f"Mark as {status}",
                command=lambda s=status, i=item: self._update_status(i, s),
            )
        menu.tk_popup(event.x_root, event.y_root)

    def _copy_id(self, memory_id):
        self.root.clipboard_clear()
        self.root.clipboard_append(memory_id)
        self._set_status(f"Copied {memory_id}")

    def _update_status(self, memory_id, status):
        try:
            self.service.update_memory(memory_id, {"status": status})
            self._cards_cache.pop(memory_id, None)
            self._set_status(f"Updated {memory_id} -> {status}")
            self._refresh()
        except Exception as exc:
            messagebox.showerror("Update Error", str(exc))

    def _show_detail(self, memory_id):
        self._selected_id = memory_id
        try:
            data = self.service.open_memory(memory_id)
        except MemoryNotFoundError:
            self._set_status(f"Memory not found: {memory_id}")
            return
        except Exception as exc:
            self._set_status(f"Error: {exc}")
            return
        self._cards_cache[memory_id] = data
        self._render_detail(data)

    def _render_detail(self, data):
        self.detail_text.configure(state=tk.NORMAL)
        self.detail_text.delete("1.0", tk.END)

        self.detail_title_label.configure(text=data["title"])

        status = data["status"]
        meta = (
            f"ID: {data['id']}  |  "
            f"Type: {data['type']}  |  "
            f"Status: {_status_icon(status)} {status}  |  "
            f"Confidence: {data['confidence']:.0%}  |  "
            f"Updated: {data['updated_at'][:16].replace('T', ' ')}"
        )
        self.detail_meta_label.configure(text=meta)

        spacer = "-" * 52

        sections = [
            ("Summary", data.get("summary")),
            ("Content", data.get("content")),
        ]

        for label, value in sections:
            self.detail_text.insert(tk.END, f"{label}\n", "field")
            self.detail_text.insert(tk.END, f"{spacer}\n", "separator")
            if value:
                wrapped = textwrap.fill(value, width=74)
                self.detail_text.insert(tk.END, f"{wrapped}\n\n", "value")
            else:
                self.detail_text.insert(tk.END, "(empty)\n\n", "empty")

        source = data.get("source", {})
        self.detail_text.insert(tk.END, "Source\n", "field")
        self.detail_text.insert(tk.END, f"{spacer}\n", "separator")
        src_text = f"Kind: {source.get('kind', '?')}\n{source.get('description', '')}"
        self.detail_text.insert(tk.END, f"{src_text}\n\n", "value")

        tags = data.get("tags", [])
        self.detail_text.insert(tk.END, "Tags\n", "field")
        self.detail_text.insert(tk.END, f"{spacer}\n", "separator")
        if tags:
            self.detail_text.insert(tk.END, ", ".join(tags) + "\n\n", "value")
        else:
            self.detail_text.insert(tk.END, "(none)\n\n", "empty")

        scope = data.get("scope", {})
        self.detail_text.insert(tk.END, "Scope\n", "field")
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
            ("\n".join(scope_parts) if scope_parts else "(none)") + "\n\n",
            "value" if scope_parts else "empty",
        )

        relations = data.get("relations", {})
        self.detail_text.insert(tk.END, "Relations\n", "field")
        self.detail_text.insert(tk.END, f"{spacer}\n", "separator")
        rel_parts = []
        for rel, targets in relations.items():
            if targets:
                rel_parts.append(f"{rel}: {', '.join(targets)}")
        self.detail_text.insert(
            tk.END,
            ("\n".join(rel_parts) if rel_parts else "(none)") + "\n\n",
            "value" if rel_parts else "empty",
        )

        self.detail_text.configure(state=tk.DISABLED)

    def _clear_detail(self):
        self._selected_id = None
        self.detail_title_label.configure(text="Select a memory")
        self.detail_meta_label.configure(text="")
        self.detail_text.configure(state=tk.NORMAL)
        self.detail_text.delete("1.0", tk.END)
        self.detail_text.configure(state=tk.DISABLED)

    def _set_status(self, text):
        self.status_label.configure(text=text)

    def run(self):
        self._start_tray()
        self.root.mainloop()


def main():
    parser = argparse.ArgumentParser(
        prog="pmem-gui",
        description="Desktop GUI for Project Memory Palace",
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