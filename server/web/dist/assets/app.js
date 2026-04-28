const app = document.querySelector("#app");

const viewConfig = {
  inbox: {
    label: "Inbox",
    subtitle: "Tasks waiting to be organized",
    empty: "Nothing in Inbox right now.",
  },
  today: {
    label: "Today",
    subtitle: "A calm view of what matters next",
    empty: "Nothing scheduled for Today.",
  },
  upcoming: {
    label: "Upcoming",
    subtitle: "Tasks with future dates and deadlines",
    empty: "No upcoming tasks right now.",
  },
  anytime: {
    label: "Anytime",
    subtitle: "Tasks ready when you are",
    empty: "Nothing in Anytime right now.",
  },
  someday: {
    label: "Someday",
    subtitle: "Ideas and maybes parked for later",
    empty: "Nothing in Someday right now.",
  },
};

const viewOrder = ["inbox", "today", "upcoming", "anytime", "someday"];
const initialLocation = locationStateFromPath(window.location.pathname);

const state = {
  view: initialLocation.view,
  viewUUID: initialLocation.uuid,
  viewMeta: null,
  tasks: [],
  counts: {},
  selectedTask: null,
  checklist: [],
  authRequired: false,
  authenticated: false,
  areas: [],
  projects: [],
  tags: [],
  areaMap: new Map(),
  projectMap: new Map(),
  tagMap: new Map(),
  lastLoadedAt: null,
  banner: "",
  createDraft: {
    open: false,
    title: "",
    submitting: false,
    error: "",
  },
};

function locationStateFromPath(pathname) {
  const path = pathname.replace(/^\/+|\/+$/g, "");
  if (!path) {
    return { view: "today", uuid: "" };
  }
  if (viewConfig[path]) {
    return { view: path, uuid: "" };
  }

  const segments = path.split("/");
  if (segments.length === 2) {
    if (segments[0] === "projects") {
      return { view: "project", uuid: segments[1] };
    }
    if (segments[0] === "areas") {
      return { view: "area", uuid: segments[1] };
    }
  }

  return { view: "today", uuid: "" };
}

function escapeHtml(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

function pluralize(count, singular, plural = `${singular}s`) {
  return `${count} ${count === 1 ? singular : plural}`;
}

function defaultCreateWhen(view) {
  return {
    inbox: "inbox",
    today: "today",
    anytime: "anytime",
    someday: "someday",
    upcoming: "anytime",
    project: "",
    area: "",
  }[view] || "inbox";
}

function viewForWhen(when) {
  return {
    inbox: "inbox",
    today: "today",
    anytime: "anytime",
    someday: "someday",
  }[when] || state.view;
}

function createContextForView(view) {
  switch (view) {
    case "inbox":
      return {
        label: "Inbox",
        detail: "This task starts in Inbox.",
      };
    case "today":
      return {
        label: "Today",
        detail: "This task will appear in Today. Today is a state, not its permanent home.",
      };
    case "anytime":
      return {
        label: "Anytime",
        detail: "This task starts in Anytime.",
      };
    case "someday":
      return {
        label: "Someday",
        detail: "This task starts in Someday.",
      };
    case "upcoming":
      return {
        label: "Upcoming",
        detail: "Upcoming is a date view. New tasks start in Anytime for now.",
      };
    case "project": {
      const title = state.viewMeta ? taskTitle(state.viewMeta) : "Project";
      const area = state.viewMeta ? state.areaMap.get(areaId(state.viewMeta)) : null;
      return {
        label: title,
        detail: area
          ? `This task starts in ${title}, inside ${area.Title || area.title}.`
          : `This task starts in ${title}.`,
      };
    }
    case "area": {
      const title = state.viewMeta?.Title || state.viewMeta?.title || "Area";
      return {
        label: title,
        detail: `This task starts in ${title}.`,
      };
    }
    default:
      return {
        label: "Inbox",
        detail: "This task starts in Inbox.",
      };
  }
}

function pathForView(view, viewUUID = "") {
  if (view === "today") {
    return "/";
  }
  if (viewConfig[view]) {
    return `/${view}`;
  }
  if (view === "project" && viewUUID) {
    return `/projects/${viewUUID}`;
  }
  if (view === "area" && viewUUID) {
    return `/areas/${viewUUID}`;
  }
  return "/";
}

function setLocationForView(view, viewUUID = "") {
  const nextPath = pathForView(view, viewUUID);
  if (window.location.pathname !== nextPath) {
    window.history.pushState({ view, uuid: viewUUID }, "", nextPath);
  }
}

function tasksPathForView(view, viewUUID = "") {
  if (view === "project" && viewUUID) {
    return `/api/projects/${viewUUID}/tasks`;
  }
  if (view === "area" && viewUUID) {
    return `/api/areas/${viewUUID}/tasks`;
  }
  return `/api/tasks/${view}`;
}

async function fetchViewMeta(view, viewUUID = "") {
  if (view === "project" && viewUUID) {
    return fetchJson(`/api/tasks/${viewUUID}`);
  }
  if (view === "area" && viewUUID) {
    return fetchJson(`/api/areas/${viewUUID}`);
  }
  return null;
}

function currentViewConfig() {
  if (viewConfig[state.view]) {
    return viewConfig[state.view];
  }

  if (state.view === "project") {
    const area = state.viewMeta ? state.areaMap.get(areaId(state.viewMeta)) : null;
    return {
      label: state.viewMeta ? taskTitle(state.viewMeta) : "Project",
      subtitle: area ? `In ${area.Title || area.title}` : "Tasks in this project",
      empty: "No tasks in this project yet.",
    };
  }

  if (state.view === "area") {
    return {
      label: state.viewMeta?.Title || state.viewMeta?.title || "Area",
      subtitle: "Tasks in this area",
      empty: "No tasks in this area yet.",
    };
  }

  return viewConfig.today;
}

function createDraftState(view, overrides = {}) {
  return {
    open: false,
    title: "",
    submitting: false,
    error: "",
    ...overrides,
  };
}

function taskId(task) {
  return task?.UUID || task?.uuid || "";
}

function taskTitle(task) {
  return task?.Title || task?.title || "(untitled)";
}

function taskNote(task) {
  return task?.Note || task?.note || "";
}

function taskStatus(task) {
  if (typeof task?.status === "string") {
    return task.status;
  }
  if (task?.Status === 1) {
    return "completed";
  }
  if (task?.Status === 2) {
    return "canceled";
  }
  return "open";
}

function taskType(task) {
  if (typeof task?.type === "string") {
    return task.type;
  }
  if (task?.Type === 1) {
    return "project";
  }
  if (task?.Type === 2) {
    return "heading";
  }
  return "task";
}

function isHeading(task) {
  return taskType(task) === "heading";
}

function isCompleted(task) {
  return taskStatus(task) === "completed";
}

function parentId(task) {
  return task?.ProjectID || task?.project_id || task?.ParentTaskIDs?.[0] || "";
}

function areaId(task) {
  return task?.AreaID || task?.area_id || task?.AreaIDs?.[0] || "";
}

function taskTags(task) {
  return task?.Tags || task?.tags || task?.TagIDs || [];
}

function scheduledDate(task) {
  return task?.ScheduledDate || task?.scheduled_for || task?.ScheduledFor || "";
}

function deadlineDate(task) {
  return task?.DeadlineDate || task?.deadline || task?.Deadline || "";
}

function todayIndexReference(task) {
  return task?.TodayIndexReference || task?.today_index_ref || task?.TodayIndexRef || "";
}

function createdDate(task) {
  return task?.CreationDate || "";
}

function scheduleValue(task) {
  if (typeof task?.schedule === "number") {
    return task.schedule;
  }
  if (typeof task?.Schedule === "number") {
    return task.Schedule;
  }
  return null;
}

function isInboxTask(task) {
  return scheduleValue(task) === 0;
}

function checklistTitle(item) {
  return item?.Title || item?.title || "(untitled)";
}

function checklistCompleted(item) {
  if (typeof item?.status === "string") {
    return item.status === "completed";
  }
  return item?.Status === 1;
}

function formatDate(dateString, options) {
  if (!dateString) {
    return "";
  }

  const value = new Date(dateString);
  if (Number.isNaN(value.getTime())) {
    return "";
  }

  return new Intl.DateTimeFormat(undefined, options).format(value);
}

function isCurrentUTCDay(dateString) {
  if (!dateString) {
    return false;
  }

  const value = new Date(dateString);
  if (Number.isNaN(value.getTime())) {
    return false;
  }

  const now = new Date();
  return (
    value.getUTCFullYear() === now.getUTCFullYear() &&
    value.getUTCMonth() === now.getUTCMonth() &&
    value.getUTCDate() === now.getUTCDate()
  );
}

function isTaskInToday(task) {
  return isCurrentUTCDay(todayIndexReference(task)) || isCurrentUTCDay(scheduledDate(task));
}

function formatTodayHeading() {
  return new Intl.DateTimeFormat(undefined, {
    weekday: "long",
    month: "long",
    day: "numeric",
  }).format(new Date());
}

function relativeCreatedAt(task) {
  const dateString = createdDate(task);
  if (!dateString) {
    return "";
  }

  const created = new Date(dateString);
  if (Number.isNaN(created.getTime())) {
    return "";
  }

  const diffMs = Date.now() - created.getTime();
  const diffDays = Math.max(0, Math.floor(diffMs / 86400000));

  if (diffDays === 0) {
    return "Added today";
  }
  if (diffDays === 1) {
    return "Added yesterday";
  }
  return `Added ${diffDays} days ago`;
}

async function fetchJson(url, options = {}) {
  const response = await fetch(url, {
    headers: {
      "Content-Type": "application/json",
      ...(options.headers || {}),
    },
    ...options,
  });

  const text = await response.text();
  const data = text ? JSON.parse(text) : {};

  if (!response.ok) {
    const error = new Error(data.error || `Request failed: ${response.status}`);
    error.status = response.status;
    throw error;
  }

  return data;
}

function indexReferenceData() {
  state.areaMap = new Map(state.areas.map((area) => [area.UUID || area.uuid, area]));
  state.projectMap = new Map(state.projects.map((project) => [project.UUID || project.uuid, project]));
  state.tagMap = new Map(state.tags.map((tag) => [tag.UUID || tag.uuid, tag]));
}

async function loadSession() {
  const session = await fetchJson("/api/auth/session");
  state.authenticated = Boolean(session.authenticated);
  state.authRequired = Boolean(session.auth_required);
}

async function loadReferenceData() {
  state.areas = await fetchJson("/api/areas");
  state.projects = await fetchJson("/api/projects");
  state.tags = await fetchJson("/api/tags");
  indexReferenceData();
}

async function fetchViewCounts(activeView, activeTasks) {
  const nextCounts = {};
  if (viewOrder.includes(activeView)) {
    nextCounts[activeView] = Array.isArray(activeTasks) ? activeTasks.length : 0;
  }

  await Promise.all(
    viewOrder.map(async (view) => {
      if (view === activeView && viewOrder.includes(activeView)) {
        return;
      }

      try {
        const tasks = await fetchJson(tasksPathForView(view));
        nextCounts[view] = Array.isArray(tasks) ? tasks.length : 0;
      } catch (_error) {
        nextCounts[view] = null;
      }
    }),
  );

  return nextCounts;
}

async function fetchTaskDetail(uuid) {
  const [task, checklist] = await Promise.all([
    fetchJson(`/api/tasks/${uuid}`),
    fetchJson(`/api/tasks/${uuid}/checklist`),
  ]);

  return {
    task,
    checklist: Array.isArray(checklist) ? checklist : [],
  };
}

async function applyViewState(view, options = {}) {
  const viewUUID = options.viewUUID === undefined ? state.viewUUID : options.viewUUID;
  state.view = view;
  state.viewUUID = viewUUID || "";
  if (options.pushHistory !== false) {
    setLocationForView(view, state.viewUUID);
  }

  const selectedTaskId = options.selectedTaskId === undefined
    ? (state.selectedTask ? taskId(state.selectedTask) : "")
    : options.selectedTaskId;

  const tasksPromise = fetchJson(tasksPathForView(view, state.viewUUID));
  const countsPromise = tasksPromise.then((tasks) => fetchViewCounts(view, tasks));
  const metaPromise = fetchViewMeta(view, state.viewUUID);
  const detailPromise = tasksPromise.then((tasks) => {
    if (!selectedTaskId || !tasks.some((task) => taskId(task) === selectedTaskId)) {
      return null;
    }
    return fetchTaskDetail(selectedTaskId);
  });

  const [tasks, counts, detail, meta] = await Promise.all([tasksPromise, countsPromise, detailPromise, metaPromise]);

  state.tasks = tasks;
  state.viewMeta = meta;
  Object.assign(state.counts, counts);
  state.lastLoadedAt = new Date();
  document.title = `Ting - ${currentViewConfig().label}`;

  if (detail) {
    state.selectedTask = detail.task;
    state.checklist = detail.checklist;
  } else {
    state.selectedTask = null;
    state.checklist = [];
  }

  renderApp();
}

async function loadView(view, options = {}) {
  await applyViewState(view, options);
}

async function loadTask(uuid) {
  state.createDraft = createDraftState(state.view);
  const detail = await fetchTaskDetail(uuid);
  state.selectedTask = detail.task;
  state.checklist = detail.checklist;
  renderApp();
}

async function triageTask(uuid, action) {
  await fetchJson(`/api/tasks/${action}`, {
    method: "POST",
    body: JSON.stringify({ uuid }),
  });

  state.banner =
    {
      "move-to-today": "Task also appears in Today now.",
      "remove-from-today": "Task no longer appears in Today.",
      "move-to-anytime": "Task moved to Anytime.",
    }[action] || "Task updated.";

  await applyViewState(state.view, {
    pushHistory: false,
    viewUUID: state.viewUUID,
    selectedTaskId: state.selectedTask ? taskId(state.selectedTask) : "",
  });
}

async function handleLogout() {
  await fetchJson("/api/auth/logout", { method: "POST" });
  state.selectedTask = null;
  state.checklist = [];
  await boot();
}

function renderLogin(errorMessage = "") {
  app.innerHTML = `
    <div class="center-panel">
      <div class="login-container">
        <div class="logo">
          <h1>Ting</h1>
          <p>A calm place for your tasks</p>
        </div>
        <form class="login-form" id="login-form">
          <div class="form-group">
            <label for="password">Password</label>
            <input
              id="password"
              name="password"
              type="password"
              placeholder="Enter your password"
              autocomplete="current-password"
            />
          </div>
          <button class="submit-btn" type="submit">Sign in</button>
          ${errorMessage ? `<p class="error">${escapeHtml(errorMessage)}</p>` : ""}
        </form>
        <div class="footer">Ting - your Things Cloud, everywhere</div>
      </div>
    </div>
  `;

  document.querySelector("#login-form").addEventListener("submit", async (event) => {
    event.preventDefault();
    const password = document.querySelector("#password").value;

    try {
      await fetchJson("/api/auth/login", {
        method: "POST",
        body: JSON.stringify({ password }),
      });
      await boot();
    } catch (error) {
      renderLogin(error.message || "Sign-in failed");
    }
  });
}

function renderViewDate() {
  if (state.view !== "today") {
    return "";
  }
  return `<span class="view-date">${escapeHtml(formatTodayHeading())}</span>`;
}

function renderViewCount(view) {
  if (view === "project" || view === "area") {
    return `<span class="view-count">${escapeHtml(pluralize(state.tasks.length, "task"))}</span>`;
  }
  const count = state.counts[view];
  if (typeof count !== "number") {
    return "";
  }
  return `<span class="view-count">${escapeHtml(pluralize(count, "task"))}</span>`;
}

function renderSidebarViews() {
  return `
    <div class="sidebar-section">
      <div class="sidebar-section-title">Views</div>
      <ul class="sidebar-nav">
        ${viewOrder
          .map((view) => {
            const config = viewConfig[view];
            const count = state.counts[view];
            return `
              <li>
                <button
                  class="sidebar-link ${state.view === view ? "is-active" : ""}"
                  data-route-kind="view"
                  data-route-view="${view}"
                >
                  <span>${escapeHtml(config.label)}</span>
                  <span class="count">${typeof count === "number" ? escapeHtml(count) : ""}</span>
                </button>
              </li>
            `;
          })
          .join("")}
      </ul>
    </div>
  `;
}

function renderSidebarLists() {
  if (!state.areas.length && !state.projects.length) {
    return "";
  }

  const groupedProjects = new Map();
  const looseProjects = [];

  for (const project of state.projects) {
    const currentAreaId = project.AreaIDs?.[0];
    if (currentAreaId) {
      if (!groupedProjects.has(currentAreaId)) {
        groupedProjects.set(currentAreaId, []);
      }
      groupedProjects.get(currentAreaId).push(project);
    } else {
      looseProjects.push(project);
    }
  }

  return `
    <div class="sidebar-section is-separated">
      <ul class="sidebar-nav">
        ${looseProjects
          .slice(0, 4)
          .map(
            (project) => `
              <li>
                <button
                  class="sidebar-link sidebar-project ${state.view === "project" && state.viewUUID === (project.UUID || project.uuid) ? "is-active" : ""}"
                  type="button"
                  data-route-kind="project"
                  data-route-uuid="${escapeHtml(project.UUID || project.uuid || "")}"
                >
                  <span>${escapeHtml(project.Title || project.title || "(untitled project)")}</span>
                </button>
              </li>
            `,
          )
          .join("")}
        ${state.areas
          .map((area) => {
            const projects = groupedProjects.get(area.UUID || area.uuid) || [];
            return `
              <li>
                <button
                  class="sidebar-link sidebar-area ${state.view === "area" && state.viewUUID === (area.UUID || area.uuid) ? "is-active" : ""}"
                  type="button"
                  data-route-kind="area"
                  data-route-uuid="${escapeHtml(area.UUID || area.uuid || "")}"
                >${escapeHtml(area.Title || area.title)}</button>
              </li>
              ${projects
                .slice(0, 6)
                .map(
                  (project) => `
                    <li>
                      <button
                        class="sidebar-link sidebar-project ${state.view === "project" && state.viewUUID === (project.UUID || project.uuid) ? "is-active" : ""}"
                        type="button"
                        data-route-kind="project"
                        data-route-uuid="${escapeHtml(project.UUID || project.uuid || "")}"
                      >
                        <span>${escapeHtml(project.Title || project.title || "(untitled project)")}</span>
                      </button>
                    </li>
                  `,
                )
                .join("")}
            `;
          })
          .join("")}
      </ul>
    </div>
  `;
}

function renderSidebarTags() {
  if (!state.tags.length) {
    return "";
  }

  return `
    <div class="sidebar-section is-separated">
      <div class="sidebar-section-title">Tags</div>
      <ul class="sidebar-nav">
        ${state.tags
          .slice(0, 8)
          .map(
            (tag) => `
              <li>
                <button class="sidebar-link sidebar-tag is-disabled" type="button">
                  <span>${escapeHtml(tag.Title || tag.title)}</span>
                </button>
              </li>
            `,
          )
          .join("")}
      </ul>
    </div>
  `;
}

function renderSidebarFooter() {
  const footerLines = [];

  if (state.lastLoadedAt) {
    footerLines.push(
      `Last refreshed ${escapeHtml(
        new Intl.DateTimeFormat(undefined, {
          hour: "numeric",
          minute: "2-digit",
        }).format(state.lastLoadedAt),
      )}`,
    );
  } else {
    footerLines.push("Connected to Things Cloud");
  }

  footerLines.push(escapeHtml(window.location.host));

  return `
    <div class="sidebar-footer">
      <div>${footerLines.join("<br />")}</div>
      ${
        state.authRequired && state.authenticated
          ? '<button class="sidebar-logout" type="button" data-logout="true">Sign out</button>'
          : ""
      }
    </div>
  `;
}

function taskMetaPieces(task, taskIndex) {
  const pieces = [];
  const currentParentId = parentId(task);
  const currentAreaId = areaId(task);
  const hideProject = state.view === "project" && state.viewUUID === currentParentId;
  const hideArea = state.view === "area" && state.viewUUID === currentAreaId;

  if (state.view !== "today" && isTaskInToday(task)) {
    pieces.push('<span class="tag tag-today">Today</span>');
  }

  if (currentParentId && !hideProject) {
    const project = state.projectMap.get(currentParentId);
    const parentTask = taskIndex.get(currentParentId);
    if (project) {
      pieces.push(`<span class="project-name">${escapeHtml(project.Title || project.title)}</span>`);
    } else if (parentTask) {
      pieces.push(`<span class="project-name">${escapeHtml(taskTitle(parentTask))}</span>`);
    }
  }

  if (currentAreaId && !hideArea) {
    const area = state.areaMap.get(currentAreaId);
    if (area) {
      pieces.push(`<span>${escapeHtml(area.Title || area.title)}</span>`);
    }
  }

  const scheduled = formatDate(scheduledDate(task), {
    month: "short",
    day: "numeric",
  });
  if (scheduled && state.view !== "today") {
    pieces.push(`<span>${escapeHtml(scheduled)}</span>`);
  }

  const deadline = formatDate(deadlineDate(task), {
    month: "short",
    day: "numeric",
  });
  if (deadline) {
    pieces.push(`<span class="deadline">Due ${escapeHtml(deadline)}</span>`);
  }

  const created = relativeCreatedAt(task);
  if (created && state.view === "inbox") {
    pieces.push(`<span>${escapeHtml(created)}</span>`);
  }

  for (const tagId of taskTags(task).slice(0, 3)) {
    const tag = state.tagMap.get(tagId);
    if (tag) {
      pieces.push(`<span class="tag">${escapeHtml(tag.Title || tag.title)}</span>`);
    }
  }

  return pieces;
}

function quickActionsForTask(task) {
  switch (state.view) {
    case "inbox":
      return [
        isTaskInToday(task)
          ? { label: "Remove Today", action: "remove-from-today" }
          : { label: "Today", action: "move-to-today" },
        { label: "Anytime", action: "move-to-anytime" },
      ];
    case "today":
      if (isInboxTask(task)) {
        return [{ label: "Remove Today", action: "remove-from-today" }];
      }
      return [{ label: "Anytime", action: "move-to-anytime" }];
    case "anytime":
    case "upcoming":
    case "someday":
    case "project":
    case "area":
      if (isTaskInToday(task)) {
        return [{ label: "Remove Today", action: "remove-from-today" }];
      }
      return [{ label: "Today", action: "move-to-today" }];
    default:
      return [];
  }
}

function renderTaskItem(task, taskIndex) {
  const uuid = taskId(task);
  const selected = state.selectedTask && taskId(state.selectedTask) === uuid;
  const meta = taskMetaPieces(task, taskIndex);
  const actions = quickActionsForTask(task);

  return `
    <div
      class="task-item ${selected ? "is-selected" : ""} ${isCompleted(task) ? "is-completed" : ""}"
      data-task-open="${escapeHtml(uuid)}"
      role="button"
      tabindex="0"
    >
      <span class="checkbox" aria-hidden="true"></span>
      <span class="task-content">
        <span class="task-title">${escapeHtml(taskTitle(task))}</span>
        ${meta.length ? `<span class="task-meta">${meta.join('<span class="separator">&middot;</span>')}</span>` : ""}
      </span>
      ${
        actions.length
          ? `
            <span class="task-actions">
              ${actions
                .map(
                  (item) => `
                    <button
                      class="task-action-btn"
                      type="button"
                      data-task-action="${item.action}"
                      data-task-uuid="${escapeHtml(uuid)}"
                    >${escapeHtml(item.label)}</button>
                  `,
                )
                .join("")}
            </span>
          `
          : ""
      }
    </div>
  `;
}

function buildTaskGroups(tasks) {
  const groups = [];
  let current = { title: "", tasks: [] };

  for (const task of tasks) {
    if (isHeading(task)) {
      if (current.title || current.tasks.length) {
        groups.push(current);
      }
      current = { title: taskTitle(task), tasks: [] };
      continue;
    }
    current.tasks.push(task);
  }

  if (current.title || current.tasks.length || !groups.length) {
    groups.push(current);
  }

  return groups.filter((group) => group.title || group.tasks.length);
}

function renderTaskList() {
  const taskIndex = new Map(state.tasks.map((task) => [taskId(task), task]));
  const groups = state.tasks.length
    ? buildTaskGroups(state.tasks)
    : [];
  const config = currentViewConfig();

  return `
    ${state.tasks.length
      ? `
        <div class="task-list">
          ${groups
            .map((group) => {
              const items = group.tasks.map((task) => renderTaskItem(task, taskIndex)).join("");
              return `
                <section class="task-group">
                  ${group.title ? `<h2 class="task-group-title">${escapeHtml(group.title)}</h2>` : ""}
                  ${items}
                </section>
              `;
            })
            .join("")}
        </div>
      `
      : `
        <div class="empty-state">
          <p>${escapeHtml(config.empty)}</p>
        </div>
      `}
  `;
}

function renderCreateFab() {
  return `
    <div class="create-floating">
      <button class="create-fab" type="button" data-open-create-fab="true" aria-label="Create task">
        <span class="create-fab-mark">+</span>
      </button>
    </div>
  `;
}

function renderCreatePanel() {
  const context = createContextForView(state.view);

  return `
    <div class="detail-overlay open">
      <div class="detail-backdrop" data-detail-close="true"></div>
      <aside class="detail-panel detail-panel-create">
        <button class="detail-close" type="button" data-detail-close="true" aria-label="Close detail panel">&times;</button>
        <p class="detail-eyebrow">New Task</p>
        <h2 class="detail-title">Capture it quickly</h2>
        <p class="detail-copy create-intro">Drop it where it belongs now. You can add notes, tags, and structure after it exists.</p>
        <form class="create-panel-form" id="new-task-form">
          <section class="detail-section create-panel-section is-first">
            <label class="create-panel-label" for="new-task-input">Title</label>
            <input
              class="create-input"
              id="new-task-input"
              name="title"
              type="text"
              placeholder="What needs doing?"
              value="${escapeHtml(state.createDraft.title)}"
              autocomplete="off"
            />
          </section>
          <section class="detail-section create-panel-section">
            <div class="create-panel-label">Context</div>
            <div class="create-context">
              <span class="create-context-chip">${escapeHtml(context.label)}</span>
              <p class="create-meta">${escapeHtml(context.detail)}</p>
            </div>
          </section>
          ${state.createDraft.error ? `<div class="create-error">${escapeHtml(state.createDraft.error)}</div>` : ""}
          <div class="create-actions">
            <button class="create-submit" type="submit" ${state.createDraft.submitting ? "disabled" : ""}>
              ${state.createDraft.submitting ? "Adding..." : "Create task"}
            </button>
            <button class="create-cancel" type="button" data-cancel-create="true">Cancel</button>
          </div>
        </form>
      </aside>
    </div>
  `;
}

function renderDetailMeta() {
  if (!state.selectedTask) {
    return "";
  }

  const detail = [];
  const task = state.selectedTask;
  const currentParentId = parentId(task);
  const currentAreaId = areaId(task);
  const project = currentParentId ? state.projectMap.get(currentParentId) : null;
  const area = currentAreaId ? state.areaMap.get(currentAreaId) : null;

  if (project) {
    detail.push(`<span class="detail-chip">${escapeHtml(project.Title || project.title)}</span>`);
  }
  if (area) {
    detail.push(`<span class="detail-chip">${escapeHtml(area.Title || area.title)}</span>`);
  }
  if (isTaskInToday(task)) {
    detail.push('<span class="detail-chip detail-chip-today">Today</span>');
  }

  const scheduled = formatDate(scheduledDate(task), {
    month: "long",
    day: "numeric",
    year: "numeric",
  });
  if (scheduled) {
    detail.push(`<span class="detail-chip">Scheduled ${escapeHtml(scheduled)}</span>`);
  }

  const deadline = formatDate(deadlineDate(task), {
    month: "long",
    day: "numeric",
    year: "numeric",
  });
  if (deadline) {
    detail.push(`<span class="detail-chip">Due ${escapeHtml(deadline)}</span>`);
  }

  for (const tagId of taskTags(task)) {
    const tag = state.tagMap.get(tagId);
    if (tag) {
      detail.push(`<span class="detail-chip">${escapeHtml(tag.Title || tag.title)}</span>`);
    }
  }

  return detail.length ? `<div class="detail-meta">${detail.join("")}</div>` : "";
}

function renderChecklist() {
  if (!state.checklist.length) {
    return '<p class="detail-copy is-muted">No checklist items.</p>';
  }

  return `
    <ul class="checklist">
      ${state.checklist
        .map(
          (item) => `
            <li class="checklist-item ${checklistCompleted(item) ? "is-completed" : ""}">
              <span class="checkbox" aria-hidden="true"></span>
              <span>${escapeHtml(checklistTitle(item))}</span>
            </li>
          `,
        )
        .join("")}
    </ul>
  `;
}

function renderDetailActions() {
  if (!state.selectedTask) {
    return "";
  }

  const uuid = taskId(state.selectedTask);
  const actions = quickActionsForTask(state.selectedTask).map((action, index) => `
    <button
      class="detail-action ${index === 0 ? "is-primary" : ""}"
      type="button"
      data-task-action="${action.action}"
      data-task-uuid="${escapeHtml(uuid)}"
    >
      ${escapeHtml(action.label)}
    </button>
  `);

  if (!actions.length) {
    return "";
  }

  return `<div class="detail-actions">${actions.join("")}</div>`;
}

function renderDetailPanel() {
  if (state.createDraft.open) {
    return renderCreatePanel();
  }

  if (!state.selectedTask) {
    return "";
  }

  return `
    <div class="detail-overlay open">
      <div class="detail-backdrop" data-detail-close="true"></div>
      <aside class="detail-panel">
        <button class="detail-close" type="button" data-detail-close="true" aria-label="Close detail panel">&times;</button>
        <h2 class="detail-title">${escapeHtml(taskTitle(state.selectedTask))}</h2>
        ${renderDetailMeta()}
        ${renderDetailActions()}
        <section class="detail-section">
          <h3>Notes</h3>
          <p class="detail-copy ${taskNote(state.selectedTask) ? "" : "is-muted"}">${
            taskNote(state.selectedTask)
              ? escapeHtml(taskNote(state.selectedTask))
              : "No notes yet."
          }</p>
        </section>
        <section class="detail-section">
          <h3>Checklist</h3>
          ${renderChecklist()}
        </section>
      </aside>
    </div>
  `;
}

function renderBanner() {
  if (!state.banner) {
    return "";
  }
  return `<div class="status-banner">${escapeHtml(state.banner)}</div>`;
}

function renderApp() {
  const config = currentViewConfig();

  app.innerHTML = `
    <div class="app-shell">
      <nav class="sidebar">
        <div class="sidebar-brand">
          <h1>Ting</h1>
        </div>
        ${renderSidebarViews()}
        ${renderSidebarLists()}
        ${renderSidebarTags()}
        <div class="sidebar-spacer"></div>
        ${renderSidebarFooter()}
      </nav>
      <main class="main">
        <div class="content">
          <div class="view-header">
            <div class="view-header-main">
              <span class="view-title">${escapeHtml(config.label)}</span>
              ${renderViewCount(state.view)}
            </div>
            ${renderViewDate()}
          </div>
          <p class="view-subtitle">${escapeHtml(config.subtitle)}</p>
          ${renderBanner()}
          ${renderTaskList()}
        </div>
      </main>
      ${renderDetailPanel()}
      ${renderCreateFab()}
    </div>
  `;

  document.querySelectorAll("[data-route-kind]").forEach((button) => {
    button.addEventListener("click", () => {
      const nextView = button.dataset.routeKind === "view" ? button.dataset.routeView : button.dataset.routeKind;
      const nextUUID = button.dataset.routeUuid || "";
      state.banner = "";
      state.selectedTask = null;
      state.checklist = [];
      state.createDraft = createDraftState(nextView);
      loadView(nextView, { viewUUID: nextUUID });
    });
  });

  document.querySelectorAll("[data-task-open]").forEach((button) => {
    button.addEventListener("click", () => loadTask(button.dataset.taskOpen));
    button.addEventListener("keydown", (event) => {
      if (event.key === "Enter" || event.key === " ") {
        event.preventDefault();
        loadTask(button.dataset.taskOpen);
      }
    });
  });

  document.querySelectorAll("[data-task-action]").forEach((button) => {
    button.addEventListener("click", (event) => {
      event.stopPropagation();
      triageTask(button.dataset.taskUuid, button.dataset.taskAction);
    });
  });

  document.querySelectorAll("[data-detail-close]").forEach((button) => {
    button.addEventListener("click", () => {
      state.selectedTask = null;
      state.checklist = [];
      state.createDraft = createDraftState(state.view);
      renderApp();
    });
  });

  const logout = document.querySelector("[data-logout='true']");
  if (logout) {
    logout.addEventListener("click", () => handleLogout());
  }

  const openCreate = document.querySelector("[data-open-create-fab='true']");
  if (openCreate) {
    openCreate.addEventListener("click", () => {
      state.selectedTask = null;
      state.checklist = [];
      state.createDraft = state.createDraft.open
        ? {
            ...state.createDraft,
            error: "",
          }
        : createDraftState(state.view, { open: true });
      renderApp();
      const input = document.querySelector("#new-task-input");
      if (input) {
        input.focus();
      }
    });
  }

  const cancelCreate = document.querySelector("[data-cancel-create='true']");
  if (cancelCreate) {
    cancelCreate.addEventListener("click", () => {
      state.createDraft = createDraftState(state.view);
      renderApp();
    });
  }

  const createForm = document.querySelector("#new-task-form");
  if (createForm) {
    createForm.addEventListener("submit", handleCreateTaskSubmit);
    const input = document.querySelector("#new-task-input");
    if (input) {
      input.addEventListener("input", (event) => {
        state.createDraft.title = event.target.value;
        state.createDraft.error = "";
      });
      if (state.createDraft.open) {
        input.focus();
        input.setSelectionRange(input.value.length, input.value.length);
      }
    }
  }
}

async function handleCreateTaskSubmit(event) {
  event.preventDefault();

  const title = state.createDraft.title.trim();
  if (!title) {
    state.createDraft.error = "Title is required.";
    renderApp();
    const input = document.querySelector("#new-task-input");
    if (input) {
      input.focus();
    }
    return;
  }

  state.createDraft.submitting = true;
  state.createDraft.error = "";
  renderApp();

  try {
    const createPayload = { title };
    switch (state.view) {
      case "inbox":
        createPayload.when = "inbox";
        break;
      case "today":
        createPayload.when = "today";
        break;
      case "anytime":
        createPayload.when = "anytime";
        break;
      case "someday":
        createPayload.when = "someday";
        break;
      case "upcoming":
        createPayload.when = "anytime";
        break;
      case "project":
        createPayload.project = state.viewUUID;
        break;
      case "area":
        createPayload.area = state.viewUUID;
        break;
      default:
        createPayload.when = defaultCreateWhen(state.view);
        break;
    }

    const created = await fetchJson("/api/tasks/create", {
      method: "POST",
      body: JSON.stringify(createPayload),
    });

    state.banner = `Created "${created.title || title}".`;
    const nextView = state.view === "upcoming" ? viewForWhen("anytime") : state.view;
    const nextUUID = state.view === "upcoming" ? "" : state.viewUUID;
    state.createDraft = createDraftState(nextView);

    await applyViewState(nextView, { viewUUID: nextUUID, selectedTaskId: created.uuid || "" });
  } catch (error) {
    state.createDraft.submitting = false;
    state.createDraft.error = error.message || "Could not create task.";
    renderApp();
    const input = document.querySelector("#new-task-input");
    if (input) {
      input.focus();
      input.setSelectionRange(input.value.length, input.value.length);
    }
  }
}

async function boot() {
  try {
    await loadSession();
    if (state.authRequired && !state.authenticated) {
      renderLogin();
      return;
    }

    await loadReferenceData();
    await loadView(state.view, { pushHistory: false, viewUUID: state.viewUUID });
  } catch (error) {
    app.innerHTML = `
      <div class="center-panel">
        <div class="login-container">
          <div class="logo">
            <h1>Ting</h1>
            <p>A calm place for your tasks</p>
          </div>
          <p class="error">${escapeHtml(error.message || "Failed to load the app")}</p>
        </div>
      </div>
    `;
  }
}

window.addEventListener("popstate", async () => {
  const nextLocation = locationStateFromPath(window.location.pathname);
  if (nextLocation.view === state.view && nextLocation.uuid === state.viewUUID) {
    return;
  }

  state.selectedTask = null;
  state.checklist = [];
  state.createDraft = createDraftState(nextLocation.view);
  await loadView(nextLocation.view, { pushHistory: false, viewUUID: nextLocation.uuid });
});

boot();
