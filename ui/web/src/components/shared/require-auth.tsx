import { Navigate, useLocation } from "react-router";
import { useAuthStore } from "@/stores/use-auth-store";
import { ROUTES } from "@/lib/constants";

function AuthLoader() {
  return (
    <div className="flex h-dvh items-center justify-center">
      <div className="h-6 w-6 animate-spin rounded-full border-2 border-muted-foreground border-t-transparent" />
    </div>
  );
}

export function RequireAuth({ children }: { children: React.ReactNode }) {
  const token = useAuthStore((s) => s.token);
  const userId = useAuthStore((s) => s.userId);
  const senderID = useAuthStore((s) => s.senderID);
  const connected = useAuthStore((s) => s.connected);
  const tenantSelected = useAuthStore((s) => s.tenantSelected);
  const availableTenants = useAuthStore((s) => s.availableTenants);
  const tenantsLoaded = useAuthStore((s) => s.tenantsLoaded);
  const isOwner = useAuthStore((s) => s.isOwner);
  const location = useLocation();

  if ((!token && !senderID) || !userId) {
    return <Navigate to={ROUTES.LOGIN} state={{ from: location }} replace />;
  }

  if (connected && !tenantsLoaded) {
    return <AuthLoader />;
  }

  if (connected && tenantsLoaded && !tenantSelected && availableTenants.length > 0) {
    return <Navigate to={ROUTES.SELECT_TENANT} state={{ from: location }} replace />;
  }

  if (connected && tenantsLoaded && !tenantSelected && availableTenants.length === 0 && !isOwner) {
    return <Navigate to={ROUTES.SELECT_TENANT} replace />;
  }

  return <>{children}</>;
}
