import { useEffect } from "react";
import { useNavigate } from "react-router";
import { useAuth, useCanRegister } from "../lib/auth";

export default function Index() {
  const { user, loading } = useAuth();
  const canRegister = useCanRegister();
  const navigate = useNavigate();

  useEffect(() => {
    if (loading) return;
    if (user) {
      navigate("/projects", { replace: true });
      return;
    }
    if (canRegister === null) return;
    navigate(canRegister ? "/setup" : "/login", { replace: true });
  }, [user, loading, canRegister, navigate]);

  return null;
}
