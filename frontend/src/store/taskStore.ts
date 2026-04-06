import {create} from "zustand/index";

type session_id = string;
type TaskState = {
    data: session_id | null;
    setData: (id: session_id | null) => void;
    resetData: () => void;
}
export const TaskStore = create<TaskState>((set) => ({
    data: null,
    setData: (data) => set({data}),
    resetData: () => set({data: null})
}))