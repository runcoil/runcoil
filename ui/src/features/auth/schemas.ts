import { z } from "zod";

export const loginSchema = z.object({
  login: z.string().trim().min(1, "Enter your username or email"),
  password: z.string().min(1, "Enter your password"),
});

export type LoginValues = z.infer<typeof loginSchema>;

export const passwordSchema = z
  .string()
  .refine((value) => Array.from(value).length >= 8, "Use at least 8 characters")
  .regex(/\p{Lu}/u, "Use an upper-case letter")
  .regex(/\p{Ll}/u, "Use a lower-case letter")
  .regex(/\p{Nd}/u, "Use a digit")
  .regex(/[^\p{L}\p{Nd}]/u, "Use a special character");

export const changePasswordSchema = z
  .object({
    currentPassword: z.string().min(1, "Enter your current password"),
    newPassword: passwordSchema,
    confirmPassword: z.string(),
  })
  .refine((values) => values.newPassword === values.confirmPassword, {
    message: "The passwords do not match",
    path: ["confirmPassword"],
  });

export type ChangePasswordValues = z.infer<typeof changePasswordSchema>;
