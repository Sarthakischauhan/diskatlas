#define _XOPEN_SOURCE 700
#include <curses.h>
#include <dirent.h>
#include <errno.h>
#include <limits.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <sys/statvfs.h>
#include <locale.h>
#include <poll.h>
#include <fcntl.h>
#include <unistd.h>

#ifndef PATH_MAX
#define PATH_MAX 4096
#endif

typedef struct { char *name; unsigned long long kb; int dir; int error; } Entry;
static Entry *entries;
static size_t count;
static char current[PATH_MAX];
static int cancelled;
static int quit_requested;
static int scanning;
static char message[256];

static void clear_entries(void) {
    for (size_t i = 0; i < count; i++) free(entries[i].name);
    free(entries); entries = NULL; count = 0;
}

/* du reports allocated disk blocks, does not follow symlinks, and -x keeps
   recursive walks on the same filesystem. Never pass paths through a shell. */
static unsigned long long size_of(const char *path, int *error) {
    int fd[2];
    if (pipe(fd) != 0) { *error = 1; return 0; }
    pid_t pid = fork();
    if (pid == -1) { close(fd[0]); close(fd[1]); *error = 1; return 0; }
    if (pid == 0) {
        close(fd[0]); dup2(fd[1], STDOUT_FILENO); close(fd[1]);
        FILE *devnull = freopen("/dev/null", "w", stderr); (void)devnull;
        execl("/usr/bin/du", "du", "-skx", path, (char *)NULL);
        _exit(127);
    }
    close(fd[1]);
    fcntl(fd[0], F_SETFL, O_NONBLOCK);
    char buf[128] = {0}; size_t used = 0;
    int status = 0, done = 0;
    while (!done) {
        struct pollfd p = {fd[0], POLLIN | POLLHUP, 0};
        (void)poll(&p, 1, 100);
        char chunk[4096]; ssize_t n;
        while ((n = read(fd[0], chunk, sizeof(chunk))) > 0) {
            size_t copy = (size_t)n < sizeof(buf) - 1 - used ? (size_t)n : sizeof(buf) - 1 - used;
            memcpy(buf + used, chunk, copy); used += copy; buf[used] = '\0';
        }
        pid_t result = waitpid(pid, &status, WNOHANG);
        if (result == pid) done = 1;
        else if (result < 0 && errno != EINTR) { *error = 1; done = 1; }
        if (scanning) {
            int key = getch();
            if (key == 'q' || key == 27) {
                cancelled = 1; quit_requested = key == 'q';
                kill(pid, SIGTERM);
            }
        }
    }
    close(fd[0]);
    unsigned long long size = 0;
    if (sscanf(buf, "%llu", &size) != 1 || !WIFEXITED(status) || WEXITSTATUS(status) != 0) *error = 1;
    return size;
}

static int compare(const void *a, const void *b) {
    const Entry *x = a, *y = b;
    if (x->kb != y->kb) return x->kb < y->kb ? 1 : -1;
    return strcmp(x->name, y->name);
}

static int scan(void) {
    DIR *d = opendir(current);
    if (!d) return -1;
    clear_entries(); cancelled = 0; scanning = 1; message[0] = '\0';
    nodelay(stdscr, TRUE);
    struct dirent *item;
    while ((item = readdir(d))) {
        if (!strcmp(item->d_name, ".") || !strcmp(item->d_name, "..")) continue;
        char path[PATH_MAX];
        if (snprintf(path, sizeof(path), "%s/%s", current, item->d_name) >= (int)sizeof(path)) continue;
        struct stat st;
        if (lstat(path, &st) != 0) continue;
        Entry *tmp = realloc(entries, (count + 1) * sizeof(*entries));
        if (!tmp) { closedir(d); scanning = 0; nodelay(stdscr, FALSE); return -1; }
        entries = tmp;
        Entry *e = &entries[count];
        e->name = strdup(item->d_name);
        if (!e->name) { closedir(d); scanning = 0; nodelay(stdscr, FALSE); return -1; }
        e->dir = S_ISDIR(st.st_mode); e->error = 0;
        erase(); mvprintw(0, 0, "DISK ATLAS  /  Scanning");
        mvaddnstr(2, 0, current, getmaxx(stdscr) - 1);
        mvprintw(4, 0, "%zu items scanned  |  Esc cancel  |  q quit", count);
        mvaddnstr(6, 0, item->d_name, getmaxx(stdscr) - 1); refresh();
        e->kb = size_of(path, &e->error); count++;
        if (cancelled) break;
    }
    closedir(d); scanning = 0; nodelay(stdscr, FALSE);
    if (cancelled) snprintf(message, sizeof(message), "Scan cancelled: results are incomplete. Press r to retry.");
    qsort(entries, count, sizeof(*entries), compare); return 0;
}

static void human(unsigned long long kb, char *out, size_t n) {
    double v = (double)kb; const char *units[] = {"KiB", "MiB", "GiB", "TiB"};
    int i = 0; while (v >= 1024 && i < 3) { v /= 1024; i++; }
    snprintf(out, n, "%7.1f %s", v, units[i]);
}

/* Binary treemap: split each group near half its weight, alternating axes.
   Every entry has a minimum weight of one so empty folders remain selectable. */
static void treemap(size_t first, size_t last, int y, int x, int h, int w, size_t selected) {
    if (first >= last || h < 1 || w < 1) return;
    if (last - first == 1) {
        Entry *e = &entries[first];
        if (has_colors()) attron(COLOR_PAIR((int)(first % 6) + 1));
        if (first == selected) attron(A_REVERSE | A_BOLD);
        for (int row = y; row < y + h; row++) {
            move(row, x);
            for (int col = 0; col < w; col++) addch(' ');
        }
        if (w >= 4) {
            move(y, x + 1); addnstr(e->name, w - 2);
            if (h >= 2) { char label[40]; human(e->kb, label, sizeof(label)); move(y + 1, x + 1); addnstr(label, w - 2); }
            if (h >= 3 && e->error) { move(y + 2, x + 1); addnstr("! incomplete", w - 2); }
        }
        if (first == selected) attroff(A_REVERSE | A_BOLD);
        if (has_colors()) attroff(COLOR_PAIR((int)(first % 6) + 1));
        return;
    }
    unsigned long long total = 0, part = 0;
    for (size_t i = first; i < last; i++) total += entries[i].kb ? entries[i].kb : 1;
    size_t split = first;
    do { part += entries[split].kb ? entries[split].kb : 1; split++; } while (split < last - 1 && part < total / 2);
    if (w >= h * 2 && w >= 3) {
        int n = (int)((double)w * (double)part / (double)total);
        if (n < 1) n = 1;
        if (n >= w) n = w - 1;
        treemap(first, split, y, x, h, n, selected);
        treemap(split, last, y, x + n, h, w - n, selected);
    } else if (h >= 2) {
        int n = (int)((double)h * (double)part / (double)total);
        if (n < 1) n = 1;
        if (n >= h) n = h - 1;
        treemap(first, split, y, x, n, w, selected);
        treemap(split, last, y + n, x, h - n, w, selected);
    } else if (w >= 2) {
        int n = (int)((double)w * (double)part / (double)total);
        if (n < 1) n = 1;
        if (n >= w) n = w - 1;
        treemap(first, split, y, x, h, n, selected);
        treemap(split, last, y, x + n, h, w - n, selected);
    }
}

int main(int argc, char **argv) {
    if (argc > 2) { fprintf(stderr, "Usage: %s [directory]\n", argv[0]); return 2; }
    const char *start = argc == 2 ? argv[1] : getenv("HOME");
    if (!start || !realpath(start, current)) { perror("directory"); return 1; }
    if (!isatty(STDIN_FILENO) || !isatty(STDOUT_FILENO)) { fprintf(stderr, "A terminal is required.\n"); return 1; }
    setlocale(LC_ALL, "");
    initscr(); cbreak(); noecho(); keypad(stdscr, TRUE); curs_set(0);
    if (has_colors()) {
        start_color();
        init_pair(1, COLOR_WHITE, COLOR_BLUE); init_pair(2, COLOR_BLACK, COLOR_CYAN);
        init_pair(3, COLOR_BLACK, COLOR_GREEN); init_pair(4, COLOR_WHITE, COLOR_MAGENTA);
        init_pair(5, COLOR_BLACK, COLOR_YELLOW); init_pair(6, COLOR_WHITE, COLOR_RED);
    }
    size_t selected = 0;
    int failed = scan() != 0;
    for (;;) {
        if (quit_requested) break;
        erase(); int rows, cols; getmaxyx(stdscr, rows, cols);
        if (rows < 8 || cols < 20) { mvaddnstr(0, 0, "Terminal too small; q quits", cols - 1); refresh(); if (getch() == 'q') break; continue; }
        attron(A_BOLD); mvaddnstr(0, 0, " DISK ATLAS  |  allocated disk usage", cols - 1); attroff(A_BOLD);
        mvaddnstr(1, 0, current, cols - 1);
        struct statvfs fs;
        char space[128] = "Filesystem usage unavailable";
        if (statvfs(current, &fs) == 0) {
            char available[32], total[32];
            human((unsigned long long)fs.f_bavail * fs.f_frsize / 1024, available, sizeof(available));
            human((unsigned long long)fs.f_blocks * fs.f_frsize / 1024, total, sizeof(total));
            snprintf(space, sizeof(space), "%s free of %s", available, total);
        }
        mvaddnstr(2, 0, message[0] ? message : space, cols - 1);
        if (count) {
            char size[32], info[PATH_MAX + 100]; human(entries[selected].kb, size, sizeof(size));
            snprintf(info, sizeof(info), "%zu/%zu  %s  %s%s%s", selected + 1, count, size,
                     entries[selected].name, entries[selected].dir ? "/" : "", entries[selected].error ? "  ! incomplete" : "");
            mvaddnstr(3, 0, info, cols - 1);
            treemap(0, count, 4, 0, rows - 5, cols - 1, selected);
        } else mvaddnstr(3, 0, failed ? "Cannot read directory; r retries" : "Empty directory", cols - 1);
        mvaddnstr(rows - 1, 0, "Up/down j/k: select  Enter: open  Backspace: parent  r: refresh  q: quit", cols - 1);
        refresh(); int ch = getch();
        if (ch == 'q') break;
        if ((ch == KEY_DOWN || ch == 'j') && selected + 1 < count) selected++;
        else if ((ch == KEY_UP || ch == 'k') && selected > 0) selected--;
        else if ((ch == '\n' || ch == KEY_ENTER || ch == KEY_RIGHT) && count && entries[selected].dir) {
            char path[PATH_MAX], resolved[PATH_MAX];
            if (snprintf(path, sizeof(path), "%s/%s", current, entries[selected].name) < (int)sizeof(path) && realpath(path, resolved)) {
                DIR *d = opendir(resolved);
                if (d) { closedir(d); strcpy(current, resolved); selected = 0; failed = scan() != 0; }
                else { failed = 1; snprintf(message, sizeof(message), "Cannot open directory: %s", strerror(errno)); }
            }
        } else if (ch == KEY_BACKSPACE || ch == 127 || ch == 8 || ch == KEY_LEFT) {
            char *slash = strrchr(current, '/');
            if (slash && slash != current) *slash = '\0'; else strcpy(current, "/");
            selected = 0; failed = scan() != 0;
        } else if (ch == 'r') { selected = 0; failed = scan() != 0; }
    }
    endwin(); clear_entries(); return 0;
}
