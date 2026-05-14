import discordLogoURL from "../../assets/discord.svg";
import githubLogoURL from "../../assets/github.svg";

const providerIconURL: Record<string, string> = {
  discord: discordLogoURL,
  github: githubLogoURL,
};

interface ProviderIconProps {
  provider: string;
  className?: string;
}

export function ProviderIcon({ provider, className = "" }: ProviderIconProps): JSX.Element | null {
  const normalizedProvider = provider.toLowerCase().trim();
  const iconURL = providerIconURL[normalizedProvider];
  if (!iconURL) {
    return null;
  }

  return (
    <img
      className={`provider-icon provider-icon-${normalizedProvider} ${className}`.trim()}
      src={iconURL}
      alt=""
      aria-hidden="true"
    />
  );
}
