import { zodResolver } from "@hookform/resolvers/zod";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate, useRouter } from "@tanstack/react-router";
import { useState } from "react";
import { Controller, useForm } from "react-hook-form";

import { Button } from "@/components/ui/button";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { isUnauthenticatedError } from "@/lib/api-errors";
import { setServerErrors } from "@/lib/form-errors";
import { DefaultPasswordPrompt } from "@/features/auth/components/default-password-prompt";
import { useLogin } from "@/features/auth/queries";
import { loginSchema, type LoginValues } from "@/features/auth/schemas";

export function LoginForm({ redirectTo }: { redirectTo: string }) {
  const queryClient = useQueryClient();
  const router = useRouter();
  const navigate = useNavigate();
  const login = useLogin();
  const [passwordPromptOpen, setPasswordPromptOpen] = useState(false);

  function changePasswordFromPrompt() {
    setPasswordPromptOpen(false);
    void navigate({ to: "/account" });
  }

  function skipPasswordPrompt() {
    setPasswordPromptOpen(false);
    void router.navigate({ href: redirectTo });
  }

  const form = useForm<LoginValues>({
    resolver: zodResolver(loginSchema),
    defaultValues: { login: "", password: "" },
  });

  function onSubmit(values: LoginValues) {
    login.mutate(
      { body: values },
      {
        onSuccess: (data) => {
          queryClient.clear();
          if (data.passwordChangeSuggested) {
            setPasswordPromptOpen(true);
            return;
          }
          void router.navigate({ href: redirectTo });
        },
        onError: (error) => {
          if (isUnauthenticatedError(error)) {
            form.setError("root", { message: "Wrong username or password" });
            return;
          }
          setServerErrors(
            form.setError,
            error,
            ["login", "password"],
            "Sign in failed. Try again.",
          );
        },
      },
    );
  }

  return (
    <>
      <form onSubmit={form.handleSubmit(onSubmit)} noValidate>
        <FieldGroup>
          <Controller
            name="login"
            control={form.control}
            render={({ field, fieldState }) => (
              <Field data-invalid={fieldState.invalid}>
                <FieldLabel htmlFor={field.name}>Username or email</FieldLabel>
                <Input
                  {...field}
                  id={field.name}
                  autoComplete="username"
                  autoFocus
                  aria-invalid={fieldState.invalid}
                />
                {fieldState.invalid && <FieldError errors={[fieldState.error]} />}
              </Field>
            )}
          />
          <Controller
            name="password"
            control={form.control}
            render={({ field, fieldState }) => (
              <Field data-invalid={fieldState.invalid}>
                <FieldLabel htmlFor={field.name}>Password</FieldLabel>
                <Input
                  {...field}
                  id={field.name}
                  type="password"
                  autoComplete="current-password"
                  aria-invalid={fieldState.invalid}
                />
                {fieldState.invalid && <FieldError errors={[fieldState.error]} />}
              </Field>
            )}
          />
          {form.formState.errors.root && <FieldError errors={[form.formState.errors.root]} />}
          <Button type="submit" className="w-full" disabled={login.isPending}>
            Sign in
          </Button>
        </FieldGroup>
      </form>
      <DefaultPasswordPrompt
        open={passwordPromptOpen}
        onChangePassword={changePasswordFromPrompt}
        onSkip={skipPasswordPrompt}
      />
    </>
  );
}
