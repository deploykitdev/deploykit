import { type RouteConfig, index, route } from "@react-router/dev/routes";

export default [
  index("routes/_index.tsx"),
  route("login", "routes/login.tsx"),
  route("register", "routes/register.tsx"),
  route("projects", "routes/projects.tsx"),
  route("projects/:id", "routes/project.tsx"),
] satisfies RouteConfig;
