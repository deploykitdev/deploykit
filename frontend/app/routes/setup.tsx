import { useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router";
import { SetupLayout } from "@/components/setup-layout";
import { AccountStep } from "@/components/setup/account-step";
import { Stepper } from "@/components/setup/stepper";
import { WelcomeStep } from "@/components/setup/welcome-step";
import { api } from "../lib/api";
import { useAuth, useCanRegister } from "../lib/auth";

export default function Setup() {
  const { user, loading, login } = useAuth();
  const canRegister = useCanRegister();
  const navigate = useNavigate();
  const [step, setStep] = useState(0);
  const prevStep = useRef(0);
  const direction = step >= prevStep.current ? "forward" : "back";

  useEffect(() => {
    prevStep.current = step;
  }, [step]);

  useEffect(() => {
    if (!loading && user) {
      navigate("/projects", { replace: true });
    }
  }, [user, loading, navigate]);

  useEffect(() => {
    if (canRegister === false && !user) {
      navigate("/login", { replace: true });
    }
  }, [canRegister, user, navigate]);

  if (loading || user || canRegister === null) {
    return (
      <SetupLayout>
        <p className="text-center text-muted-foreground">Loading...</p>
      </SetupLayout>
    );
  }

  if (!canRegister) return null;

  async function handleCreateAccount(values: {
    name: string;
    email: string;
    password: string;
  }) {
    await api("/auth/register", {
      method: "POST",
      body: JSON.stringify(values),
    });
    await login(values.email, values.password);
    navigate("/projects", { replace: true });
  }

  const totalSteps = 2;

  const slideIn =
    direction === "forward"
      ? "animate-in fade-in slide-in-from-right-4 duration-300"
      : "animate-in fade-in slide-in-from-left-4 duration-300";

  return (
    <SetupLayout>
      <div className="flex flex-col gap-12">
        <Stepper total={totalSteps} current={step} />
        <div key={step} className={slideIn}>
          {step === 0 && <WelcomeStep onNext={() => setStep(1)} />}
          {step === 1 && (
            <AccountStep
              onSubmit={handleCreateAccount}
              onBack={() => setStep(0)}
            />
          )}
        </div>
      </div>
    </SetupLayout>
  );
}
