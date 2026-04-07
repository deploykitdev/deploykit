import { type RouteConfig, index, route } from "@react-router/dev/routes";

export default [
  index("routes/_index.tsx"),
  route("login", "routes/login.tsx"),
  route("register", "routes/register.tsx"),
  route("projects", "routes/projects.tsx"),
  route("projects/:id", "routes/project.tsx"),
  route("profile", "routes/profile.tsx"),
  route("settings", "routes/settings.tsx", [
    index("routes/settings/general.tsx"),
    route("users", "routes/settings/users.tsx"),
    route("system", "routes/settings/system.tsx"),
    route("about", "routes/settings/about.tsx"),
  ]),
] satisfies RouteConfig;
