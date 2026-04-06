
import { create } from 'zustand';

interface RiskEditState {
    riskData: {
        id?: string;
        riskId?: string;
        contractType?: string;
        applicableScope?: 'individual' | 'department' | 'platform';
        department?: string[];
        riskContent?: string;
        isEnabled?: 'enabled' | 'disabled';
    };
    setRiskData: (data: RiskEditState['riskData']) => void;
    clearRiskData: () => void;
}

export const useRiskEditStore = create<RiskEditState>((set) => ({
    riskData: {},
    setRiskData: (data) => set({ riskData: data }),
    clearRiskData: () => set({ riskData: {} })
}));
