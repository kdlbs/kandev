import { HeroSection } from '@/components/hero-section';
import { FeaturesSection } from '@/components/features-section';
import { AgentsSection } from '@/components/agents-section';
import { FutureFeaturesSection } from '@/components/future-features-section';

export default function HomePage() {
  return (
    <main>
      <HeroSection />
      <FeaturesSection />
      <AgentsSection />
      <FutureFeaturesSection />
    </main>
  );
}
