import { seedActivity } from "../data/activity";
import { seededQuery } from "./seeded";

export const activityQuery = seededQuery("activity", seedActivity);
