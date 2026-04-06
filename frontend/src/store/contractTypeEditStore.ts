
import { create } from 'zustand';

interface ContractTypeEditState {
    contractTypeData: {
        id?: string;
        contractTypeName?: string;
        templateContent?: string;
    };
    setContractTypeData: (data: ContractTypeEditState['contractTypeData']) => void;
    clearContractTypeData: () => void;
}

export const useContractTypeEditStore = create<ContractTypeEditState>((set) => ({
    contractTypeData: {},
    setContractTypeData: (data) => set({ contractTypeData: data }),
    clearContractTypeData: () => set({ contractTypeData: {} })
}));
